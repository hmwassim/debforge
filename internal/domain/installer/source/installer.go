package source

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/download"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
	ui     ports.UI
}

func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	if p.VersionCmd != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
	}

	tmpDir, err := i.fs.MkdirTemp("debforge-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer i.fs.RemoveAll(tmpDir)

	srcDir, err := i.getSource(ctx, p, tmpDir, spinner)
	if err != nil {
		return err
	}

	if len(p.Packages) > 0 {
		spinner.SetDesc("installing build dependencies for " + p.Name)
		if err := aptpty.RunInstall(ctx, i.runner, p.Packages, spinner); err != nil {
			return err
		}
	}

	buildScript := i.interpolate(p.BuildScript, p.Version)
	installScript := i.interpolate(p.InstallScript, p.Version)
	postinstallScript := i.interpolate(p.PostinstallScript, p.Version)

	if buildScript != "" {
		if err := installer.RunScriptInDir(ctx, i.runner, spinner, p.Name, buildScript, "building", srcDir); err != nil {
			return err
		}
	}

	if installScript == "" {
		installScript = buildScript
	}
	if installScript != "" {
		if err := installer.RunScriptInDir(ctx, i.runner, spinner, p.Name, installScript, "installing", srcDir); err != nil {
			return err
		}
	}

	if postinstallScript != "" {
		if err := installer.RunScript(ctx, i.runner, spinner, p.Name, postinstallScript, "running post-install for"); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	if p.RemoveScript != "" {
		script := i.interpolate(p.RemoveScript, p.Version)
		if err := installer.RunScript(ctx, i.runner, spinner, p.Name, script, "removing"); err != nil {
			return err
		}
	}

	pkgs := p.Packages
	if len(p.Remove) > 0 {
		pkgs = p.Remove
	}
	if len(pkgs) > 0 {
		spinner.SetDesc("removing " + p.Name + "...")
		if err := aptpty.RunRemove(ctx, i.runner, pkgs, spinner); err != nil {
			return err
		}
	}

	return nil
}

// getSource fetches the source code into tmpDir and returns the path to the
// source directory. If Repo is set (and SkipClone is false), it clones via
// git. If URL is set, it downloads and extracts a tarball. Repo takes
// priority when both are set.
func (i *Installer) getSource(ctx context.Context, p *pkg.Package, tmpDir string, spinner ports.Spinner) (string, error) {
	srcDir := filepath.Join(tmpDir, "src")

	if p.Repo != "" && !p.SkipClone {
		spinner.SetDesc("cloning " + p.Name)
		if _, _, err := i.runner.Run(ctx, "git", "clone", "--depth", "1", "--", p.Repo, srcDir); err != nil {
			return "", fmt.Errorf("clone %s: %w", p.Name, err)
		}
		return srcDir, nil
	}

	if p.URL != "" {
		spinner.SetDesc("downloading " + p.Name)
		archive := filepath.Join(tmpDir, "archive")
		url := download.ExpandURL(p.URL, p.Version)
		if err := download.Download(ctx, i.fs, url, archive, spinner, p.SHA256); err != nil {
			return "", fmt.Errorf("download %s: %w", p.Name, err)
		}

		spinner.SetDesc("extracting " + p.Name)
		if err := i.fs.MkdirAll(srcDir, 0755); err != nil {
			return "", fmt.Errorf("create src dir: %w", err)
		}
		if _, _, err := i.runner.Run(ctx, "tar", "-xf", archive, "--strip-components=1", "-C", srcDir); err != nil {
			return "", fmt.Errorf("extract %s: %w", p.Name, err)
		}
		return srcDir, nil
	}

	return "", fmt.Errorf("source definition %s: no repo or url configured", p.Name)
}

func (i *Installer) interpolate(script, version string) string {
	return strings.ReplaceAll(script, "{version}", version)
}

func (i *Installer) checkVersion(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (bool, error) {
	out, _, err := i.runner.Run(ctx, "sh", "-c", p.VersionCmd)
	if err != nil {
		return false, fmt.Errorf("version check %s: %w", p.Name, err)
	}
	latest := strings.TrimSpace(string(out))
	if latest == "" {
		return false, fmt.Errorf("version check %s: empty output", p.Name)
	}
	if latest == p.Version {
		spinner.SetDesc(p.Name + " already up to date")
		return false, nil
	}
	p.Version = latest
	return true, nil
}
