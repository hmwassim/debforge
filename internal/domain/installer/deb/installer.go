// Package deb implements installer.Installer for deb-type packages (deb
// archives downloaded from a URL and installed via apt-get).
package deb

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/download"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/installer/version"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer installs and removes .deb packages.
type Installer struct {
	runner  ports.CommandRunner
	fs      ports.FileSystem
	ui      ports.UI
	sys     ports.System
	execApt aptpty.AptExecFunc
}

// NewInstaller returns a new deb Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI, sys ports.System) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, sys: sys, execApt: aptpty.AptExec}
}

// Install downloads the .deb file(s) from p.URLs, installs them via apt-get,
// and runs the post-install script.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeDeb, "deb"); err != nil {
		return err
	}
	if len(p.URLs) == 0 {
		return fmt.Errorf("deb definition %s: no install url", p.Name)
	}

	if p.VersionCmd != "" || version.RepoFromPkg(p) != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return err
		}
		if !updated && !p.ForceInstall {
			ok, err := installer.CheckInstalled(ctx, i.runner, i.fs, i.sys, p)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}

	if len(p.Packages) > 0 {
		spinner.SetDesc("installing prerequisites for " + p.Name)
		if err := i.execApt(ctx, i.runner, append([]string{"install", "-y"}, p.Packages...), spinner); err != nil {
			return fmt.Errorf("install prerequisites %s: %w", p.Name, err)
		}
	}

	if err := installer.WithTempDir(i.fs, p.Name, func(tmpDir string) error {
		var tmpPaths []string
		for idx, raw := range p.URLs {
			url := download.ExpandURL(raw, p.Version)
			tmpPath := filepath.Join(tmpDir, download.FilenameFromURL(url))
			tmpPaths = append(tmpPaths, tmpPath)

			spinner.SetDesc("downloading " + p.Name)
			sha256 := ""
			if idx < len(p.SHA256s) {
				sha256 = p.SHA256s[idx]
			}
			if err := download.Download(ctx, i.fs, url, tmpPath, spinner, sha256); err != nil {
				return fmt.Errorf("download %s: %w", p.Name, err)
			}
		}

		spinner.SetDesc("installing " + p.Name)
		args := append([]string{"install", "-y"}, tmpPaths...)
		return i.execApt(ctx, i.runner, args, spinner)
	}); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	return nil
}

// Remove removes the system packages tracked by p via apt-get remove.
func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeDeb, "deb"); err != nil {
		return err
	}

	pkgs := p.Packages
	if len(p.Remove) > 0 {
		pkgs = p.Remove
	}
	if len(pkgs) == 0 && p.Deb != nil && p.Deb.Package != "" {
		pkgs = []string{p.Deb.Package}
	}
	if len(pkgs) == 0 {
		return nil
	}

	spinner.SetDesc("removing " + p.Name + "...")
	if err := i.execApt(ctx, i.runner, append([]string{"remove", "-y"}, pkgs...), spinner); err != nil {
		return err
	}

	if err := installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove); err != nil {
		return err
	}

	return nil
}

func (i *Installer) checkVersion(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (bool, error) {
	latest, err := version.GatherVersion(ctx, i.runner, p)
	if err != nil {
		return false, err
	}
	return version.ApplyVersionUpdate(spinner, p, latest)
}
