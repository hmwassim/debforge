package self

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

type Updater struct {
	cfg    *Config
	runner ports.CommandRunner
	fs     ports.FileSystem
	logger ports.UI
	locker ports.Locker
}

func NewUpdater(cfg *Config, runner ports.CommandRunner, fs ports.FileSystem, logger ports.UI, locker ports.Locker) *Updater {
	return &Updater{cfg: cfg, runner: runner, fs: fs, logger: logger, locker: locker}
}

func (u *Updater) Update(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--self-update must be run as root")
	}

	dirs := []string{u.cfg.BinDir, u.cfg.SourceDir, u.cfg.GoPath, u.cfg.GoCache}
	for _, d := range dirs {
		if err := u.fs.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	lockPath := filepath.Join(u.cfg.RootDir, "var", "lock")
	release, err := u.locker.Acquire(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	spinner := u.logger.Spinner(ctx, "Working")
	defer spinner.Done()

	sourceExists := sourceRepoExists(u.fs, u.cfg.SourceDir)

	if !sourceExists {
		spinner.Pause()
		u.logger.Info("debforge is not installed")
		if !u.logger.Prompt("Install debforge?") {
			u.logger.Info("Cancelled")
			return nil
		}
		spinner.Resume()

		spinner.SetDesc("Cloning repository")
		if err := u.cloneRepo(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("clone: %w", err)
		}

	} else {
		spinner.SetDesc("Checking for updates")
		if err := u.gitFetch(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("fetch: %w", err)
		}

		local, remote, err := u.compareRevisions(ctx)
		if err != nil {
			spinner.Fail()
			return fmt.Errorf("compare revisions: %w", err)
		}
		if local == remote {
			spinner.SetDesc("Already up to date")
			return nil
		}

		spinner.Pause()
		u.logger.Info("Update available")
		if !u.logger.Prompt("Update debforge?") {
			u.logger.Info("Cancelled")
			return nil
		}
		spinner.Resume()

		spinner.SetDesc("Pulling update")
		if err := u.gitPull(ctx); err != nil {
			spinner.Fail()
			return fmt.Errorf("pull: %w", err)
		}
	}

	buildPath := filepath.Join(u.cfg.BinDir, "debforge.new")

	spinner.SetDesc("Building debforge")
	if err := u.build(ctx, buildPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("build: %w", err)
	}

	spinner.SetDesc("Verifying binary")
	if err := u.verify(ctx, buildPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("verify: %w", err)
	}

	finalPath := filepath.Join(u.cfg.BinDir, "debforge")
	if err := u.installBinary(buildPath, finalPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("install binary: %w", err)
	}

	if err := u.ensureLink(finalPath, u.cfg.LinkPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("symlink: %w", err)
	}

	if sourceExists {
		spinner.SetDesc("Updated to latest version")
	} else {
		spinner.SetDesc("debforge installed")
	}
	return nil
}

func sourceRepoExists(fs ports.FileSystem, dir string) bool {
	_, err := fs.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func (u *Updater) cloneRepo(ctx context.Context) error {
	if err := u.fs.RemoveAll(u.cfg.SourceDir); err != nil {
		return err
	}
	_, _, err := u.runner.Run(ctx, "git", "clone", "-q", "--depth", "1", "--branch", u.cfg.Branch, "--", u.cfg.RepoURL, u.cfg.SourceDir)
	return err
}

func (u *Updater) gitFetch(ctx context.Context) error {
	_, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir, "fetch", "-q", "origin", u.cfg.Branch)
	return err
}

func (u *Updater) compareRevisions(ctx context.Context) (string, string, error) {
	local, err := u.gitRevParse(ctx, "HEAD")
	if err != nil {
		return "", "", err
	}
	remote, err := u.gitRevParse(ctx, "origin/"+u.cfg.Branch)
	if err != nil {
		return "", "", err
	}
	return local, remote, nil
}

func (u *Updater) gitRevParse(ctx context.Context, ref string) (string, error) {
	stdout, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (u *Updater) gitPull(ctx context.Context) error {
	if _, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir, "fetch", "--depth", "1", "origin", u.cfg.Branch); err != nil {
		return err
	}
	_, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir, "reset", "--hard", "origin/"+u.cfg.Branch)
	return err
}

func (u *Updater) build(ctx context.Context, dst string) error {
	version := "0.1.0-dev"
	if v, _, err := u.runner.Run(ctx, "git", "-C", u.cfg.SourceDir, "describe", "--tags", "--always"); err == nil {
		if s := strings.TrimSpace(string(v)); s != "" {
			version = s
		}
	}

	cmd := exec.CommandContext(ctx, u.cfg.GoBinary, "build", "-o", dst,
		"-ldflags=-X main.version="+version,
		"./cmd/debforge/")
	cmd.Dir = u.cfg.SourceDir
	env := os.Environ()
	env = append(env, "GOPATH="+u.cfg.GoPath, "GOMODCACHE="+u.cfg.GoPath+"/mod", "GOCACHE="+u.cfg.GoCache)
	cmd.Env = env
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (u *Updater) verify(ctx context.Context, path string) error {
	stdout, stderr, err := u.runner.Run(ctx, path, "--version")
	if err != nil {
		return fmt.Errorf("binary did not execute: %s", strings.TrimSpace(string(stderr)))
	}
	if len(stdout) == 0 {
		return fmt.Errorf("binary produced no output")
	}
	return nil
}

func (u *Updater) installBinary(src, dst string) error {
	data, err := u.fs.ReadFile(src)
	if err != nil {
		return err
	}
	if err := u.fs.WriteFile(dst, data, 0755); err != nil {
		return err
	}
	return u.fs.RemoveAll(src)
}

func (u *Updater) ensureLink(target, link string) error {
	fi, err := u.fs.Stat(link)
	if err != nil {
		return os.Symlink(target, link)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory", link)
	}
	current, err := os.Readlink(link)
	if err == nil && current == target {
		return nil
	}
	if err := u.fs.RemoveAll(link); err != nil {
		return err
	}
	return os.Symlink(target, link)
}
