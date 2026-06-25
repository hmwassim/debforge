package config

import (
	"context"
	"fmt"
	"os"

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
	if p.Type != pkg.TypeConfig {
		return fmt.Errorf("config installer called for type %s", p.Type)
	}

	if err := i.writeConfigs(ctx, p, spinner); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeConfig {
		return fmt.Errorf("config installer called for type %s", p.Type)
	}

	if err := installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove); err != nil {
		return err
	}

	for path := range p.RemoveConfigs {
		spinner.SetDesc("removing config " + path)
		absPath := path
		if installer.HasHomePrefix(path) {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			absPath = installer.ExpandHome(path, homeDir)
		}
		if err := i.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("remove config %s: %w", path, err)
		}
	}

	for path := range p.UserConfigs {
		spinner.SetDesc("removing user config " + path)
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		absPath := installer.ExpandHome(path, homeDir)
		if err := i.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("remove user config %s: %w", path, err)
		}
	}

	return nil
}

func (i *Installer) writeConfigs(_ context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.WriteConfigs(i.fs, spinner, p); err != nil {
		return err
	}
	return installer.WriteUserConfigs(i.fs, spinner, p)
}
