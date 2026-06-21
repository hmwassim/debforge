package source

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/utils"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	runner ports.CommandRunner
	logger ports.UI
	fs     ports.FileSystem
	tmpDir string
}

func NewInstaller(runner ports.CommandRunner, logger ports.UI, fs ports.FileSystem) *Installer {
	return NewInstallerWithTempDir(runner, logger, fs, "")
}

func NewInstallerWithTempDir(runner ports.CommandRunner, logger ports.UI, fs ports.FileSystem, tmpDir string) *Installer {
	return &Installer{runner: runner, logger: logger, fs: fs, tmpDir: tmpDir}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, _ ports.Spinner) error {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	for _, check := range p.Checks {
		_, _, err := i.runner.Run(ctx, "which", check)
		if err != nil {
			if _, _, err := i.runner.Run(ctx, "apt-get", "install", "-y", check); err != nil {
				return fmt.Errorf("%s not found in PATH and apt-get install failed", check)
			}
		}
	}

	var version string

	if p.SkipClone {
		if !p.ForceInstall && p.VersionCmd != "" {
			ver, err := i.runVersionCmd(ctx, p.VersionCmd)
			if err == nil {
				version = ver
			}
		}

		if version != "" && !p.ForceInstall && p.Version != "" && p.Version == version {
			return nil
		}

		if p.PostInstall != "" {
			if err := i.runPostCmd(ctx, p.PostInstall); err != nil {
				return fmt.Errorf("post-install: %w", err)
			}
		}

		if version == "" && p.VersionCmd != "" {
			ver, err := i.runVersionCmd(ctx, p.VersionCmd)
			if err == nil {
				version = ver
			}
		}
	} else {
		_, _, err := i.runner.Run(ctx, "which", "git")
		if err != nil {
			return fmt.Errorf("git is required to install source packages")
		}

		tmpDir, err := i.fs.MkdirTemp(i.tmpDir, "debforge-"+p.Name+"-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer i.fs.RemoveAll(tmpDir)

		if err := utils.RetryGit(ctx, func() error {
			_, _, err := i.runner.Run(ctx, "git", "clone", "--depth=1", p.Repo, tmpDir)
			return err
		}); err != nil {
			return fmt.Errorf("cloning %s: %w", p.Repo, err)
		}

		installDir := tmpDir
		if p.SourceSubdir != "" {
			installDir = filepath.Join(tmpDir, p.SourceSubdir)
		}

		if p.VersionCmd != "" {
			script := "#!/bin/sh\ncd " + shellQuote(installDir) + "\n" + p.VersionCmd + "\n"
			scriptPath, wErr := i.writeTempScript(tmpDir, ".version.sh", script)
			if wErr != nil {
				return fmt.Errorf("writing version script: %w", wErr)
			}
			stdout, stderr, err := i.runner.Run(ctx, scriptPath)
			if err == nil {
				out := stdout
				if len(out) == 0 {
					out = stderr
				}
				version = strings.TrimSpace(string(out))
			}
		} else {
			var stdout []byte
			err := utils.RetryGit(ctx, func() error {
				var err error
				stdout, _, err = i.runner.Run(ctx, "git", "-C", installDir, "rev-parse", "HEAD")
				return err
			})
			if err != nil {
				return fmt.Errorf("getting commit hash: %w", err)
			}
			version = strings.TrimSpace(string(stdout))
		}

		if !p.ForceInstall && p.Version != "" && p.Version == version {
			return nil
		}

		installScript := filepath.Join(installDir, "install.sh")
		if _, err := i.fs.Stat(installScript); err == nil {
			if _, _, err := i.runner.Run(ctx, installScript); err != nil {
				return fmt.Errorf("install.sh: %w", err)
			}
		} else {
			return fmt.Errorf("install.sh not found in repository")
		}

		if p.VersionCmd != "" && version == "" {
			if ver, err := i.runVersionCmd(ctx, p.VersionCmd); err == nil {
				version = ver
			}
		}

		if p.PostInstall != "" {
			if err := i.runPostCmd(ctx, p.PostInstall); err != nil {
				i.logger.Warn("post-install: %s", err)
			}
		}
	}

	p.Version = version
	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, _ ports.Spinner) error {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	if p.PostRemove != "" {
		if err := i.runPostCmd(ctx, p.PostRemove); err != nil {
			i.logger.Warn("post-remove: %s", err)
		}
	}

	return nil
}

func (i *Installer) Update(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	p.ForceInstall = true
	return i.Install(ctx, p, spinner)
}

func (i *Installer) runVersionCmd(ctx context.Context, cmd string) (string, error) {
	stdout, stderr, err := i.runner.Run(ctx, "sh", "-c", cmd)
	if err != nil {
		return "", err
	}
	out := stdout
	if len(out) == 0 {
		out = stderr
	}
	return strings.TrimSpace(string(out)), nil
}

func (i *Installer) writeTempScript(dir, name, content string) (string, error) {
	path := filepath.Join(dir, name)
	if err := i.fs.WriteFile(path, []byte(content), 0755); err != nil {
		return "", err
	}
	return path, nil
}

func (i *Installer) runPostCmd(ctx context.Context, script string) error {
	return utils.RunScript(ctx, i.fs, i.runner, script)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

var _ installers.Installer = (*Installer)(nil)
