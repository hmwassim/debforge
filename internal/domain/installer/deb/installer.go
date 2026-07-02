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
	execApt aptpty.AptExecFunc
}

// NewInstaller returns a new deb Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, execApt: aptpty.AptExec}
}

// Install downloads the .deb file from p.URL, installs it via apt-get,
// and runs the post-install script.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (err error) {
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
	}
	if p.URL == "" {
		return fmt.Errorf("deb definition %s: no install url", p.Name)
	}

	if p.VersionCmd != "" || version.RepoFromPkg(p) != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return err
		}
		if !updated && !p.ForceInstall {
			ok, err := installer.CheckInstalled(ctx, i.runner, i.fs, p)
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

	url := download.ExpandURL(p.URL, p.Version)

	tmpDir, err := installer.MkdirTemp(i.fs)
	if err != nil {
		return err
	}
	defer func() {
		if rmerr := i.fs.RemoveAll(tmpDir); rmerr != nil && err == nil {
			err = fmt.Errorf("clean up temp dir for %s: %w", p.Name, rmerr)
		}
	}()
	tmpPath := filepath.Join(tmpDir, download.FilenameFromURL(url))

	spinner.SetDesc("downloading " + p.Name)
	if err := download.Download(ctx, i.fs, url, tmpPath, spinner, p.SHA256); err != nil {
		return fmt.Errorf("download %s: %w", p.Name, err)
	}

	if err := i.execApt(ctx, i.runner, []string{"install", "-y", tmpPath}, spinner); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	return nil
}

// Remove removes the system packages tracked by p via apt-get remove.
func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
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
