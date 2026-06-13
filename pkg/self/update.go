package self

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/state"
	"github.com/hmwassim/debforge/pkg/text"
)

func Update(log *text.Logger) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-update must be run as root")
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if err := settings.EnsureDirsExist(); err != nil {
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

	log.Info("Building debforge...")
	buildPath := filepath.Join(settings.BinDir, "debforge")
	if err := buildBinary(buildPath); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if err := verifyBinary(buildPath); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	log.Info("Linking %s → %s", buildPath, settings.BinaryPath)
	if err := installBinary(buildPath); err != nil {
		return fmt.Errorf("linking binary: %w", err)
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
	_, err := os.Stat(filepath.Join(settings.SourceDir, ".git"))
	return err == nil
}

func cloneRepo(log *text.Logger) error {
	if _, err := os.Stat(settings.SourceDir); err == nil {
		if _, err := os.Stat(filepath.Join(settings.SourceDir, ".git")); os.IsNotExist(err) {
			log.Warn("Removing stale source directory...")
			os.RemoveAll(settings.SourceDir)
		}
	}
	log.Info("Cloning %s [branch: %s]...", settings.RepoURL, settings.Branch)
	cmd := exec.Command("git", "clone", "-q", "--depth", "1", "--branch", settings.Branch, "--", settings.RepoURL, settings.SourceDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitFetch() error {
	cmd := exec.Command("git", "fetch", "-q", "origin", settings.Branch)
	cmd.Dir = settings.SourceDir
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func compareRevisions() (local, remote string, err error) {
	local, err = gitRevParse("HEAD")
	if err != nil {
		return "", "", err
	}
	remote, err = gitRevParse("origin/" + settings.Branch)
	if err != nil {
		return "", "", err
	}
	return local, remote, nil
}

func gitRevParse(ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = settings.SourceDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitPull(log *text.Logger) error {
	log.Info("Pulling latest source...")
	cmd := exec.Command("git", "fetch", "--depth", "1", "origin", settings.Branch)
	cmd.Dir = settings.SourceDir
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "reset", "--hard", "origin/"+settings.Branch)
	cmd.Dir = settings.SourceDir
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildBinary(dst string) error {
	cmd := exec.Command("go", "build", "-o", dst, "./cmd/debforge/")
	cmd.Dir = settings.SourceDir
	cmd.Env = append(os.Environ(),
		"GOPATH="+settings.GoPathDir,
		"GOMODCACHE="+settings.GoPathDir+"/mod",
		"GOCACHE="+settings.CacheDir,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func verifyBinary(path string) error {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("binary did not execute: %w", err)
	}
	if len(out) == 0 {
		return fmt.Errorf("binary produced no output")
	}
	return nil
}

func installBinary(buildPath string) error {
	tmp := settings.BinaryPath + ".new"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(buildPath, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, settings.BinaryPath)
}

