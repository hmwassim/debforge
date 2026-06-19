package self

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	config "github.com/hmwassim/debforge/internal/config"
	"github.com/hmwassim/debforge/internal/statestore"
	"github.com/hmwassim/debforge/internal/utils"
	"github.com/hmwassim/debforge/internal/ports"
)

type debforgeState struct {
	statestore.Versioned
	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type SelfUpdater interface {
	Update(ctx context.Context) error
}

type Updater struct {
	runner ports.CommandRunner
	locker ports.Locker
	logger ports.UI
	fs     ports.FileSystem
	store  *statestore.Store
	cfg    *config.Config
}

var _ SelfUpdater = (*Updater)(nil)

func NewUpdater(runner ports.CommandRunner, locker ports.Locker, logger ports.UI, fs ports.FileSystem, cfg *config.Config) *Updater {
	return &Updater{runner: runner, locker: locker, logger: logger, fs: fs, store: statestore.New(fs), cfg: cfg}
}

func (u *Updater) Update(ctx context.Context) error {
	// BYPASS: os.Geteuid is an OS-level root check; no existing port covers it
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-update must be run as root")
	}

	if err := config.EnsureDirsExist(u.cfg, u.fs); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	release, err := u.locker.Acquire(ctx, u.cfg.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	st := &debforgeState{}
	if err := u.loadState(st); err != nil {
		u.logger.Warn("State file corrupt, resetting: %s", err)
		st = &debforgeState{}
	}

	sourceExists := u.sourceRepoExists()

	spinner := u.logger.Spinner(ctx, "Setting up debforge...")

	if !sourceExists {
		spinner.Pause()
		u.logger.Info("Debforge is not installed")
		if !u.logger.Prompt("Install debforge?") {
			u.logger.Info("Cancelled")
			spinner.Done()
			return nil
		}
		spinner.Resume()

		spinner.SetDesc("Downloading debforge...")
		if err := u.cloneRepo(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("cloning repository: %w", err)
		}
	} else {
		spinner.SetDesc("Checking for updates...")
		if err := u.gitFetch(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("fetching remote: %w", err)
		}
		localSHA, remoteSHA, err := u.compareRevisions(ctx)
		if err != nil {
			spinner.Fail()
			return fmt.Errorf("comparing revisions: %w", err)
		}
		if localSHA == remoteSHA {
			spinner.SetDesc("Already up to date")
			spinner.Done()
			return nil
		}
		spinner.Pause()
		u.logger.Info("Update available")
		if !u.logger.Prompt("Update debforge?") {
			u.logger.Info("Cancelled")
			spinner.Done()
			return nil
		}
		spinner.Resume()

		spinner.SetDesc("Downloading update...")
		if err := u.gitPull(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("pulling latest source: %w", err)
		}
	}

	buildPath := filepath.Join(u.cfg.BinDir(), "debforge.new")
	finalPath := filepath.Join(u.cfg.BinDir(), "debforge")

	spinner.SetDesc("Building debforge...")
	if err := u.buildBinary(ctx, buildPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("build failed: %w", err)
	}

	spinner.SetDesc("Verifying binary...")
	if err := u.verifyBinary(ctx, buildPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("verification failed: %w", err)
	}

	spinner.SetDesc("Installing debforge...")
	if err := u.installBinary(buildPath, finalPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("installing binary: %w", err)
	}

	if sourceExists {
		spinner.SetDesc("Updated to latest version")
	} else {
		spinner.SetDesc("Debforge installed")
	}
	spinner.Done()

	now := time.Now().UTC().Format(time.RFC3339)
	if st.InstalledAt == "" {
		st.InstalledAt = now
	}
	st.UpdatedAt = now
	if err := u.saveState(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

func (u *Updater) loadState(st *debforgeState) error {
	path := filepath.Join(u.cfg.StatesDir, "debforge.state.json")
	if err := u.store.LoadJSON(path, st); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.IsLegacy() {
		st.Version = statestore.CurrentVersion
	}
	return nil
}

func (u *Updater) saveState(st *debforgeState) error {
	st.Version = statestore.CurrentVersion
	path := filepath.Join(u.cfg.StatesDir, "debforge.state.json")
	if err := u.fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return u.store.SaveJSON(path, st)
}

func (u *Updater) sourceRepoExists() bool {
	_, err := u.fs.Stat(filepath.Join(u.cfg.SourceDir(), ".git"))
	return err == nil
}

func (u *Updater) cloneRepo(ctx context.Context) error {
	if err := u.fs.RemoveAll(u.cfg.SourceDir()); err != nil {
		return fmt.Errorf("removing existing source directory: %w", err)
	}
	_, _, err := u.runner.Run(ctx,
		"git", "clone", "-q", "--depth", "1", "--branch", u.cfg.Branch,
		"--", u.cfg.RepoURL, u.cfg.SourceDir())
	return err
}

func (u *Updater) gitFetch(ctx context.Context) error {
	return utils.RetryGit(ctx, func() error {
		_, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir(), "fetch", "-q", "origin", u.cfg.Branch)
		return err
	})
}

func (u *Updater) compareRevisions(ctx context.Context) (local, remote string, err error) {
	local, err = u.gitRevParse(ctx, "HEAD")
	if err != nil {
		return "", "", err
	}
	remote, err = u.gitRevParse(ctx, "origin/"+u.cfg.Branch)
	if err != nil {
		return "", "", err
	}
	return local, remote, nil
}

func (u *Updater) gitRevParse(ctx context.Context, ref string) (string, error) {
	var stdout string
	err := utils.RetryGit(ctx, func() error {
		out, stderr, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir(), "rev-parse", ref)
		if err != nil {
			return fmt.Errorf("git rev-parse %s: %s", ref, strings.TrimSpace(string(stderr)))
		}
		stdout = strings.TrimSpace(string(out))
		return nil
	})
	return stdout, err
}

func (u *Updater) gitPull(ctx context.Context) error {
	return utils.RetryGit(ctx, func() error {
		if _, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir(), "fetch", "--depth", "1", "origin", u.cfg.Branch); err != nil {
			return err
		}
		_, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir(), "reset", "--hard", "origin/"+u.cfg.Branch)
		return err
	})
}

func (u *Updater) gitDescribe(ctx context.Context) (string, error) {
	out, stderr, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir(), "describe", "--tags", "--always")
	if err != nil {
		return "", fmt.Errorf("git describe: %s", strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (u *Updater) buildBinary(ctx context.Context, dst string) error {
	for _, d := range []string{u.cfg.GoPathDir(), u.cfg.GoCacheDir()} {
		if err := u.fs.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	version := "0.1.0-dev"
	if v, err := u.gitDescribe(ctx); err == nil {
		version = v
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"GOPATH=" + u.cfg.GoPathDir(),
		"GOMODCACHE=" + u.cfg.GoPathDir() + "/mod",
		"GOCACHE=" + u.cfg.GoCacheDir(),
	}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "HOME", "GOFLAGS", "GOOS", "GOARCH", "GOARM", "GOAMD64",
			"CGO_ENABLED", "CC", "CXX", "GOPROXY", "GOSUMDB", "GOPRIVATE":
			env = append(env, e)
		}
	}
	_, _, err := u.runner.RunWithEnv(ctx, u.cfg.SourceDir(), env,
		u.cfg.GoBinaryPath, "build", "-o", dst,
		"-ldflags=-X github.com/hmwassim/debforge/internal/commands.Version="+version,
		"./cmd/debforge/")
	if err != nil {
		return err
	}
	if err := config.GoCacheClean(ctx, u.cfg, u.runner); err != nil {
		u.logger.Warn("cleaning go cache: %v", err)
	}
	return nil
}

func (u *Updater) verifyBinary(ctx context.Context, path string) error {
	stdout, stderr, err := u.runner.Run(ctx, path, "--version")
	if err != nil {
		return fmt.Errorf("binary did not execute: %s", strings.TrimSpace(string(stderr)))
	}
	if len(stdout) == 0 {
		return fmt.Errorf("binary produced no output")
	}
	return nil
}

func (u *Updater) installBinary(buildPath, finalPath string) error {
	if err := u.fs.Rename(buildPath, finalPath); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return err
		}
		data, err := u.fs.ReadFile(buildPath)
		if err != nil {
			return err
		}
		if err := u.fs.RemoveAll(buildPath); err != nil {
			u.logger.Warn("cleaning up build path: %v", err)
		}
		return u.fs.AtomicWriteFile(finalPath, data, 0755)
	}

	fi, err := u.fs.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("installed binary not found: %w", err)
	}
	if fi.Mode()&0100 == 0 {
		if err := u.fs.Chmod(finalPath, 0755); err != nil {
			return fmt.Errorf("setting executable bit: %w", err)
		}
	}

	return u.ensureSymlink(finalPath, u.cfg.BinaryPath)
}

func (u *Updater) ensureSymlink(target, link string) error {
	fi, err := u.fs.Lstat(link)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return u.fs.Symlink(target, link)
		}
		return fmt.Errorf("checking %s: %w", link, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s exists and is not a symlink", link)
	}
	current, err := u.fs.Readlink(link)
	if err != nil {
		return fmt.Errorf("reading symlink %s: %w", link, err)
	}
	if current == target {
		return nil
	}
	if err := u.fs.RemoveAll(link); err != nil {
		return fmt.Errorf("removing old symlink %s: %w", link, err)
	}
	return u.fs.Symlink(target, link)
}
