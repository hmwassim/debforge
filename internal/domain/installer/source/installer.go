// Package source implements installer.Installer for source-type packages
// (source code fetched from git or downloaded as a tarball, then built and
// installed via custom scripts).
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
	"github.com/hmwassim/debforge/internal/textutil"
)

// DownloadFunc downloads a file from a URL.
type DownloadFunc func(ctx context.Context, fs ports.FileSystem, url, dest string, spinner ports.Spinner, sha256 string) error

// Installer installs and removes source-built packages.
type Installer struct {
	runner       ports.CommandRunner
	fs           ports.FileSystem
	ui           ports.UI
	execApt      aptpty.AptExecFunc
	downloadFunc DownloadFunc
}

// NewInstaller returns a new source Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, execApt: aptpty.AptExec, downloadFunc: download.Download}
}

// Install fetches the source code (git clone or download+extract), runs
// the build and install scripts, then runs the post-install script.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
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

	return installer.WithTempDir(i.fs, p.Name, func(tmpDir string) error {
		srcDir, err := i.getSource(ctx, p, tmpDir, spinner)
		if err != nil {
			return err
		}

		if len(p.Packages) > 0 {
			spinner.SetDesc("installing build dependencies for " + p.Name)
			if err := i.execApt(ctx, i.runner, append([]string{"install", "-y"}, p.Packages...), spinner); err != nil {
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

		return installer.RunPostInstall(ctx, i.runner, spinner, p.Name, postinstallScript)
	})
}

// Remove runs the remove script (if defined) and removes system packages
// listed in p.Remove via apt-get.
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
		if err := i.execApt(ctx, i.runner, append([]string{"remove", "-y"}, p.Remove...), spinner); err != nil {
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
		if err := i.downloadFunc(ctx, i.fs, url, archive, spinner, p.SHA256); err != nil {
			return "", fmt.Errorf("download %s: %w", p.Name, err)
		}

		spinner.SetDesc("extracting " + p.Name)
		if err := i.fs.MkdirAll(srcDir, 0755); err != nil {
			return "", fmt.Errorf("create src dir: %w", err)
		}

		if strings.HasSuffix(p.URL, ".zip") {
			if _, _, err := i.runner.Run(ctx, "unzip", "-j", "-o", archive, "-d", srcDir); err != nil {
				return "", fmt.Errorf("extract %s: %w", p.Name, err)
			}
			return srcDir, nil
		}

		hasTopDir := false
		listing, _, err := i.runner.Run(ctx, "tar", "tf", archive)
		if err != nil {
			return "", fmt.Errorf("list archive %s: %w", p.Name, err)
		}
		for _, entry := range strings.Split(string(listing), "\n") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if strings.Contains(entry, "/") {
				hasTopDir = true
				break
			}
		}

		args := []string{"-xf", archive, "-C", srcDir}
		if hasTopDir {
			args = append(args, "--strip-components=1")
		}
		if _, _, err := i.runner.Run(ctx, "tar", args...); err != nil {
			return "", fmt.Errorf("extract %s: %w", p.Name, err)
		}
		return srcDir, nil
	}

	return "", fmt.Errorf("source definition %s: no repo or url configured", p.Name)
}

func (i *Installer) interpolate(script, version string) string {
	return textutil.ExpandVersion(script, version)
}

func (i *Installer) checkVersion(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (bool, error) {
	latest, err := version.GatherVersion(ctx, i.runner, p)
	if err != nil {
		if p.VersionCmd != "" {
			return false, err
		}
		repo := version.RepoFromPkg(p)
		if repo == "" {
			return false, err
		}
		spinner.SetDesc("checking version for " + p.Name)
		out, _, err := i.runner.Run(ctx, "git", "ls-remote", repo, "HEAD")
		if err != nil {
			return false, fmt.Errorf("version check %s: %w", p.Name, err)
		}
		if parts := strings.Fields(string(out)); len(parts) > 0 {
			latest = parts[0]
		}
	}

	return version.ApplyVersionUpdate(spinner, p, latest)
}
