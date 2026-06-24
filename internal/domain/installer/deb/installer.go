package deb

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
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
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
	}
	if p.URL == "" {
		return fmt.Errorf("deb definition %s: no install url", p.Name)
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

	url := expandURL(p.URL, p.Version)

	tmpFile, err := os.CreateTemp("", "debforge-*.deb")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	spinner.SetDesc("downloading " + p.Name)
	if err := Download(ctx, url, p.SHA256, tmpPath, spinner); err != nil {
		return fmt.Errorf("download %s: %w", p.Name, err)
	}

	spinner.SetDesc("installing " + p.Name)
	if err := aptpty.RunInstall(ctx, i.runner, []string{tmpPath}, spinner); err != nil {
		return err
	}

	if p.PostInstall != "" {
		spinner.SetDesc("running post-install for " + p.Name)
		if _, _, err := i.runner.Run(ctx, "sh", "-c", p.PostInstall); err != nil {
			return fmt.Errorf("post-install %s: %w", p.Name, err)
		}
	}

	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeDeb {
		return fmt.Errorf("deb installer called for type %s", p.Type)
	}

	pkgs := p.Remove
	if len(pkgs) == 0 && p.Package != "" {
		pkgs = []string{p.Package}
	}
	if len(pkgs) == 0 {
		return nil
	}

	spinner.SetDesc("removing " + p.Name)
	if err := aptpty.RunRemove(ctx, i.runner, pkgs, spinner); err != nil {
		return err
	}

	if p.PostRemove != "" {
		spinner.SetDesc("running post-remove for " + p.Name)
		if _, _, err := i.runner.Run(ctx, "sh", "-c", p.PostRemove); err != nil {
			return fmt.Errorf("post-remove %s: %w", p.Name, err)
		}
	}

	return nil
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
