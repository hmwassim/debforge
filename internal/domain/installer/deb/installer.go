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

type Installer struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
	ui     ports.UI
}

func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui}
}

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
			if installer.CheckInstalled(ctx, i.runner, i.fs, p) {
				return nil
			}
		}
	}

	if len(p.Packages) > 0 {
		spinner.SetDesc("installing prerequisites for " + p.Name)
		if err := aptpty.RunInstall(ctx, i.runner, p.Packages, spinner); err != nil {
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

	if err := aptpty.RunInstall(ctx, i.runner, []string{tmpPath}, spinner); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	return nil
}

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
	if err := aptpty.RunRemove(ctx, i.runner, pkgs, spinner); err != nil {
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
