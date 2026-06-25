package source

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/download"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/installer/version"
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

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (err error) {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	if p.VersionCmd != "" || version.RepoFromPkg(p) != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return err
		}
		if !updated && !p.ForceInstall {
			return nil
		}
	}

	tmpDir, err := installer.MkdirTemp(i.fs)
	if err != nil {
		return err
	}
	defer func() {
		if rmerr := i.fs.RemoveAll(tmpDir); rmerr != nil && err == nil {
			err = fmt.Errorf("clean up temp dir for %s: %w", p.Name, rmerr)
		}
	}()

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

	buildScript := i.interpolate(p.Source.BuildScript, p.Version)
	installScript := i.interpolate(p.Source.InstallScript, p.Version)
	postinstallScript := i.interpolate(p.Source.PostinstallScript, p.Version)

	if installScript == "" {
		installScript = buildScript
	}

	if buildScript != "" {
		if err := installer.RunScriptInDir(ctx, i.runner, spinner, p.Name, buildScript, "building", srcDir); err != nil {
			return err
		}
	}

	if installScript != "" && installScript != buildScript {
		if err := installer.RunScriptInDir(ctx, i.runner, spinner, p.Name, installScript, "installing", srcDir); err != nil {
			return err
		}
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, postinstallScript); err != nil {
		return err
	}

	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeSource {
		return fmt.Errorf("source installer called for type %s", p.Type)
	}

	if p.Source.RemoveScript != "" {
		script := i.interpolate(p.Source.RemoveScript, p.Version)
		if err := installer.RunScript(ctx, i.runner, spinner, p.Name, script, "removing"); err != nil {
			return err
		}
	}

	if len(p.Remove) > 0 {
		spinner.SetDesc("removing " + p.Name + "...")
		if err := aptpty.RunRemove(ctx, i.runner, p.Remove, spinner); err != nil {
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

	if p.Repo != "" && !p.Source.SkipClone {
		spinner.SetDesc("cloning " + p.Name)
		args := []string{"clone", "--depth", "1"}
		if p.Version != "" {
			prefix := p.TagPrefix
			if prefix == "" {
				prefix = "v"
			}
			args = append(args, "--branch", prefix+p.Version)
		}
		args = append(args, "--", p.Repo, srcDir)
		if _, _, err := i.runner.Run(ctx, "git", args...); err != nil {
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
	latest, err := version.GatherVersion(ctx, i.runner, p)
	if err != nil && p.VersionCmd == "" {
		if repo := version.RepoFromPkg(p); repo != "" {
			spinner.SetDesc("checking version for " + p.Name)
			out, _, err := i.runner.Run(ctx, "git", "ls-remote", repo, "HEAD")
			if err != nil {
				return false, fmt.Errorf("version check %s: %w", p.Name, err)
			}
			if parts := strings.Fields(string(out)); len(parts) > 0 {
				latest = parts[0]
			}
		}
	} else if err != nil {
		return false, err
	}

	return version.ApplyVersionUpdate(spinner, p, latest)
}
