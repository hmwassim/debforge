package self

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/state"
	"github.com/hmwassim/debforge/pkg/text"
)

func Update(log *text.Logger) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-update must be run as root")
	}

	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if err := settings.Default.EnsureDirsExist(); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	sourceExists := sourceRepoExists()

	if !sourceExists {
		log.Info("debforge is not installed")
		if !log.Prompt("Install debforge?") {
			log.Info("Cancelled")
			return nil
		}
		if err := cloneRepo(log); err != nil {
			return fmt.Errorf("cloning repository: %w", err)
		}
	} else {
		log.Info("Checking for updates...")
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
		if err := gitPull(log); err != nil {
			return fmt.Errorf("pulling latest source: %w", err)
		}
	}

	cfg := settings.Default
	buildPath := filepath.Join(cfg.BinDir(), "debforge.new")
	finalPath := filepath.Join(cfg.BinDir(), "debforge")

	log.Info("Building debforge...")
	if err := buildBinary(buildPath); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if err := verifyBinary(buildPath); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	log.Info("Installing binary...")
	if err := installBinary(buildPath, finalPath); err != nil {
		return fmt.Errorf("installing binary: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if st.InstalledAt == "" {
		st.InstalledAt = now
	}
	st.UpdatedAt = now
	if err := st.Save(); err != nil {
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

func cloneRepo(log *text.Logger) error {
	cfg := settings.Default
	if err := os.RemoveAll(cfg.SourceDir()); err != nil {
		return fmt.Errorf("removing existing source directory: %w", err)
	}
	log.Info("Cloning %s [branch: %s]...", cfg.RepoURL, cfg.Branch)
	cmd := exec.Command("git", "clone", "-q", "--depth", "1", "--branch", cfg.Branch, "--", cfg.RepoURL, cfg.SourceDir())
	cmd.Stdout = io.Discard
	return executil.Run(cmd)
}

func gitFetch() error {
	cmd := exec.Command("git", "fetch", "-q", "origin", settings.Default.Branch)
	cmd.Dir = settings.Default.SourceDir()
	cmd.Stdout = io.Discard
	return executil.Run(cmd)
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

func gitPull(log *text.Logger) error {
	cfg := settings.Default
	log.Info("Pulling latest source...")
	cmd := exec.Command("git", "fetch", "--depth", "1", "origin", cfg.Branch)
	cmd.Dir = cfg.SourceDir()
	cmd.Stdout = io.Discard
	if err := executil.Run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("git", "reset", "--hard", "origin/"+cfg.Branch)
	cmd.Dir = cfg.SourceDir()
	cmd.Stdout = io.Discard
	return executil.Run(cmd)
}

func buildBinary(dst string) error {
	cfg := settings.Default
	cmd := exec.Command("go", "build", "-o", dst, "./cmd/debforge/")
	cmd.Dir = cfg.SourceDir()
	cmd.Env = []string{
		"PATH=/usr/local/go/bin:/usr/bin:/bin",
		"GOPATH=" + cfg.GoPathDir(),
		"GOMODCACHE=" + cfg.GoPathDir() + "/mod",
		"GOCACHE=" + cfg.CacheDir(),
	}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "HOME", "GOFLAGS", "GOOS", "GOARCH", "GOARM", "GOAMD64",
			"CGO_ENABLED", "CC", "CXX", "GOPROXY", "GOSUMDB", "GOPRIVATE":
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Stdout = io.Discard
	return executil.Run(cmd)
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
		return err
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
	os.Remove(link)
	return os.Symlink(target, link)
}
