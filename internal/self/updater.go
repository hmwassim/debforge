package self

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/buildmeta"
	"github.com/hmwassim/debforge/internal/ports"
)

// Updater handles the self-update operation — fetching the latest source,
// building a new binary, and installing it.
type Updater struct {
	cfg    *Config
	runner ports.CommandRunner
	fs     ports.FileSystem
	logger ports.UI
	locker ports.Locker
	sys    ports.System
	force  bool
}

// NewUpdater returns a new Updater with the given dependencies.
func NewUpdater(cfg *Config, runner ports.CommandRunner, fs ports.FileSystem, logger ports.UI, locker ports.Locker, sys ports.System, force bool) *Updater {
	return &Updater{cfg: cfg, runner: runner, fs: fs, logger: logger, locker: locker, sys: sys, force: force}
}

// Update runs the self-update flow: clone or pull source, build, verify,
// install binary, and update the symlink.
func (u *Updater) Update(ctx context.Context) error {
	return withRootAndLock(ctx, "self-update", u.sys, u.locker, u.cfg.LockPath, u.update)
}

func (u *Updater) update(ctx context.Context) error {
	dirs := []string{u.cfg.BinDir, u.cfg.SourceDir, u.cfg.GoPath, u.cfg.GoCache}
	for _, d := range dirs {
		if err := u.fs.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	spinner := u.logger.Spinner(ctx, "Processing")
	defer spinner.Done()

	proceed, err := u.ensureSource(ctx, spinner)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	buildPath := filepath.Join(u.cfg.BinDir, "debforge.new")
	if err := u.ensureBuilt(ctx, spinner, buildPath); err != nil {
		return err
	}

	return u.ensureInstalled(ctx, spinner, buildPath)
}

// ensureSource clones (or pulls) the repository. Returns:
//
//	(true, nil)  — source is ready to build
//	(false, nil) — caller should stop (cancelled or up-to-date)
//	(_, err)     — fatal error
func (u *Updater) ensureSource(ctx context.Context, spinner ports.Spinner) (proceed bool, _ error) {
	sourceExists, err := sourceRepoExists(u.fs, u.cfg.SourceDir)
	if err != nil {
		return false, fmt.Errorf("check source dir: %w", err)
	}

	if !sourceExists {
		spinner.Pause()
		u.logger.Info("Debforge is not installed")
		if !u.logger.Prompt("Install debforge?") {
			spinner.Resume()
			spinner.SetDesc("Cancelled")
			spinner.DoneInfo()
			return false, nil
		}
		spinner.Resume()
		spinner.SetDesc("Cloning repository")
		if err := u.cloneRepo(ctx); err != nil {
			spinner.Fail()
			return false, fmt.Errorf("clone: %w", err)
		}
		return true, nil
	}

	spinner.SetDesc("Updating source")
	if err := u.gitFetch(ctx); err != nil {
		spinner.Fail()
		return false, fmt.Errorf("fetch: %w", err)
	}

	if !u.force {
		local, remote, err := u.compareRevisions(ctx)
		if err != nil {
			spinner.Fail()
			return false, fmt.Errorf("compare revisions: %w", err)
		}
		if local == remote {
			spinner.SetDesc("Already up to date")
			spinner.DoneInfo()
			return false, nil
		}
	}

	if !u.force {
		spinner.Pause()
		u.logger.Info("Update available")
		if !u.logger.Prompt("Update debforge?") {
			spinner.Resume()
			spinner.SetDesc("Cancelled")
			spinner.DoneInfo()
			return false, nil
		}
		spinner.Resume()
	}
	spinner.SetDesc("Pulling update")
	if err := u.gitPull(ctx); err != nil {
		spinner.Fail()
		return false, fmt.Errorf("pull: %w", err)
	}
	return true, nil
}

func (u *Updater) ensureBuilt(ctx context.Context, spinner ports.Spinner, buildPath string) error {
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
	return nil
}

func (u *Updater) ensureInstalled(ctx context.Context, spinner ports.Spinner, buildPath string) error {
	finalPath := filepath.Join(u.cfg.BinDir, "debforge")
	if err := u.installBinary(buildPath, finalPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("install binary: %w", err)
	}

	if err := u.ensureLink(finalPath, u.cfg.LinkPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("symlink: %w", err)
	}

	if err := u.installCompletions(ctx); err != nil {
		spinner.SetDesc("Warning: completions not installed")
		u.logger.Warn("completions: %s", err)
	}
	spinner.SetDesc("Updated to latest version")
	return nil
}

func sourceRepoExists(fs ports.FileSystem, dir string) (bool, error) {
	return fs.Exists(filepath.Join(dir, ".git"))
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
	version := buildmeta.DeriveVersion(ctx, u.runner, u.cfg.SourceDir)

	opts := ports.RunOptions{
		Dir:    u.cfg.SourceDir,
		Env:    []string{"GOPATH=" + u.cfg.GoPath, "GOMODCACHE=" + u.cfg.GoPath + "/mod", "GOCACHE=" + u.cfg.GoCache},
		Stderr: os.Stderr,
	}
	_, _, err := u.runner.RunWithOptions(ctx, opts, u.cfg.GoBinary, "build", "-o", dst,
		"-ldflags="+buildmeta.Ldflags(version),
		"./cmd/debforge/")
	return err
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
	return u.fs.Rename(src, dst)
}

func (u *Updater) installCompletions(ctx context.Context) error {
	type completion struct {
		src, dst string
	}
	entries := []completion{
		{"completions/debforge.bash", "/usr/share/bash-completion/completions/debforge"},
		{"completions/_debforge", "/usr/share/zsh/vendor-completions/_debforge"},
		{"completions/debforge.fish", "/usr/share/fish/vendor_completions.d/debforge.fish"},
	}
	for _, e := range entries {
		src := filepath.Join(u.cfg.SourceDir, e.src)
		data, err := u.fs.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", e.src, err)
		}
		if err := u.fs.MkdirAll(filepath.Dir(e.dst), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(e.dst), err)
		}
		if err := u.fs.WriteFile(e.dst, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", e.dst, err)
		}
	}
	return nil
}

func (u *Updater) ensureLink(target, link string) error {
	exists, err := u.fs.Exists(link)
	if err != nil {
		return err
	}
	if !exists {
		return u.fs.Symlink(target, link)
	}
	fi, err := u.fs.Stat(link)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory", link)
	}
	current, err := u.fs.Readlink(link)
	if err == nil && current == target {
		return nil
	}
	if err := u.fs.RemoveAll(link); err != nil {
		return err
	}
	return u.fs.Symlink(target, link)
}
