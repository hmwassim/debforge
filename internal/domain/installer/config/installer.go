package config

import (
	"context"
	"fmt"
	"path/filepath"

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

	if p.PostInstall != "" {
		if err := installer.RunScript(ctx, i.runner, spinner, p.Name, p.PostInstall, "running post-install for"); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeConfig {
		return fmt.Errorf("config installer called for type %s", p.Type)
	}

	if p.PostRemove != "" {
		if err := installer.RunScript(ctx, i.runner, spinner, p.Name, p.PostRemove, "running post-remove for"); err != nil {
			return err
		}
	}

	for path := range p.RemoveConfigs {
		spinner.SetDesc("removing config " + path)
		if err := i.fs.RemoveAll(path); err != nil {
			return fmt.Errorf("remove config %s: %w", path, err)
		}
	}

	return nil
}

func (i *Installer) writeConfigs(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if len(p.Configs) == 0 {
		return nil
	}

	spinner.SetDesc("writing configs for " + p.Name)
	for path, content := range p.Configs {
		dir := filepath.Dir(path)
		if err := i.fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
		if err := i.fs.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write config %s: %w", path, err)
		}
	}
	return nil
}
