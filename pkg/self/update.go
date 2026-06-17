package self

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/state"
	"github.com/hmwassim/debforge/pkg/text"
)

type debforgeState struct {
	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func run(cmd *exec.Cmd) error {
	cmd.Stdout = io.Discard
	return executil.Run(cmd)
}

func Update(log *text.Logger) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-update must be run as root")
	}

	if err := settings.Default.EnsureDirsExist(); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	st := &debforgeState{}
	store := state.New("debforge")
	if err := store.Load(st); err != nil {
		log.Warn("State file corrupt, resetting: %s", err)
		st = &debforgeState{}
	}

	sourceExists := sourceRepoExists()

	if !sourceExists {
		log.Info("debforge is not installed")
		if !log.Prompt("Install debforge?") {
			log.Info("Cancelled")
			return nil
		}
	} else {
		if err := gitFetch(); err != nil {
			return fmt.Errorf("fetching remote: %w", err)
		}
		localSHA, remoteSHA, err := compareRevisions()
		if err != nil {
			return fmt.Errorf("comparing revisions: %w", err)
		}
		if localSHA == remoteSHA {
			log.Success("Already up to date")
			return nil
		}
		log.Info("Update available")
		if !log.Prompt("Update debforge?") {
			log.Info("Cancelled")
			return nil
		}
	}

	cfg := settings.Default
	buildPath := filepath.Join(cfg.BinDir(), "debforge.new")
	finalPath := filepath.Join(cfg.BinDir(), "debforge")

	s := text.StartSpinner(os.Stderr, "Setting up debforge...")

	if sourceExists {
		if err := gitPull(); err != nil {
			s.Fail()
			return fmt.Errorf("pulling latest source: %w", err)
		}
	} else {
		if err := cloneRepo(); err != nil {
			s.Fail()
			return fmt.Errorf("cloning repository: %w", err)
		}
	}

	if err := buildBinary(buildPath); err != nil {
		s.Fail()
		return fmt.Errorf("build failed: %w", err)
	}

	if err := verifyBinary(buildPath); err != nil {
		s.Fail()
		return fmt.Errorf("verification failed: %w", err)
	}

	if err := installBinary(buildPath, finalPath); err != nil {
		s.Fail()
		return fmt.Errorf("installing binary: %w", err)
	}

	s.Done()

	now := time.Now().UTC().Format(time.RFC3339)
	if st.InstalledAt == "" {
		st.InstalledAt = now
	}
	st.UpdatedAt = now
	if err := store.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	if sourceExists {
		log.Success("Updated to latest version")
	} else {
		log.Success("debforge installed")
	}

	return nil
}

func sourceRepoExists() bool {
	_, err := os.Stat(filepath.Join(settings.Default.SourceDir(), ".git"))
	return err == nil
}

func cloneRepo() error {
	cfg := settings.Default
	if err := os.RemoveAll(cfg.SourceDir()); err != nil {
		return fmt.Errorf("removing existing source directory: %w", err)
	}
	cmd := exec.Command("git", "clone", "-q", "--depth", "1", "--branch", cfg.Branch, "--", cfg.RepoURL, cfg.SourceDir())
	return run(cmd)
}

func gitFetch() error {
	cmd := exec.Command("git", "fetch", "-q", "origin", settings.Default.Branch)
	cmd.Dir = settings.Default.SourceDir()
	return executil.RunWithSpinner(cmd, "Checking for updates...")
}

func compareRevisions() (local, remote string, err error) {
	local, err = gitRevParse("HEAD")
	if err != nil {
		return "", "", err
	}
	remote, err = gitRevParse("origin/" + settings.Default.Branch)
	if err != nil {
		return "", "", err
	}
	return local, remote, nil
}

func gitRevParse(ref string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = settings.Default.SourceDir()
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %s", ref, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func gitPull() error {
	cfg := settings.Default
	cmd := exec.Command("git", "fetch", "--depth", "1", "origin", cfg.Branch)
	cmd.Dir = cfg.SourceDir()
	if err := run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("git", "reset", "--hard", "origin/"+cfg.Branch)
	cmd.Dir = cfg.SourceDir()
	return run(cmd)
}

func gitDescribe() (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "describe", "--tags", "--always")
	cmd.Dir = settings.Default.SourceDir()
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git describe: %s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func buildBinary(dst string) error {
	cfg := settings.Default
	for _, d := range []string{cfg.GoPathDir(), cfg.GoCacheDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	version := "0.1.0-dev"
	if v, err := gitDescribe(); err == nil {
		version = v
	}
	cmd := exec.Command("go", "build", "-o", dst,
		"-ldflags=-X github.com/hmwassim/debforge/pkg/cli.Version="+version,
		"./cmd/debforge/")
	cmd.Dir = cfg.SourceDir()
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"GOPATH=" + cfg.GoPathDir(),
		"GOMODCACHE=" + cfg.GoPathDir() + "/mod",
		"GOCACHE=" + cfg.GoCacheDir(),
	}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "HOME", "GOFLAGS", "GOOS", "GOARCH", "GOARM", "GOAMD64",
			"CGO_ENABLED", "CC", "CXX", "GOPROXY", "GOSUMDB", "GOPRIVATE":
			cmd.Env = append(cmd.Env, e)
		}
	}
	if err := run(cmd); err != nil {
		return err
	}
	settings.Default.GoCacheClean()
	return nil
}

func verifyBinary(path string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(path, "--version")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("binary did not execute: %s", strings.TrimSpace(stderr.String()))
	}
	if len(out) == 0 {
		return fmt.Errorf("binary produced no output")
	}
	return nil
}

func installBinary(buildPath, finalPath string) error {
	if err := os.Rename(buildPath, finalPath); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return err
		}
		src, err := os.Open(buildPath)
		if err != nil {
			return err
		}
		defer func() {
			src.Close()
			os.Remove(buildPath)
		}()
		dir := filepath.Dir(finalPath)
		tmp, err := os.CreateTemp(dir, filepath.Base(finalPath))
		if err != nil {
			return err
		}
		cleanup := true
		defer func() {
			if cleanup {
				tmp.Close()
				os.Remove(tmp.Name())
			}
		}()
		if _, err := io.Copy(tmp, src); err != nil {
			return err
		}
		if err := tmp.Chmod(0755); err != nil {
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), finalPath); err != nil {
			return err
		}
		cleanup = false
	}

	// Verify the installed binary exists and is executable.
	fi, err := os.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("installed binary not found: %w", err)
	}
	if fi.Mode()&0100 == 0 {
		if err := os.Chmod(finalPath, 0755); err != nil {
			return fmt.Errorf("setting executable bit: %w", err)
		}
	}

	return ensureSymlink(finalPath, settings.Default.BinaryPath)
}

func ensureSymlink(target, link string) error {
	fi, err := os.Lstat(link)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(target, link)
	}

	// Check if it's a symlink pointing to the correct target.
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s exists and is not a symlink", link)
	}

	current, err := os.Readlink(link)
	if err != nil {
		return fmt.Errorf("reading symlink %s: %w", link, err)
	}

	if current == target {
		return nil // already correct
	}

	// Broken or incorrect symlink — replace it.
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("removing old symlink %s: %w", link, err)
	}
	return os.Symlink(target, link)
}
