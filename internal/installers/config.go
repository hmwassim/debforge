package installers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/domain/deployer"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
)

type ConfigInstaller struct {
	deployer *deployer.Deployer
	fs       ports.FileReader
	logger   ports.UI
}

func NewConfigInstaller(d *deployer.Deployer, fs ports.FileReader, logger ports.UI) *ConfigInstaller {
	return &ConfigInstaller{deployer: d, fs: fs, logger: logger}
}

func (i *ConfigInstaller) Install(ctx context.Context, p *pkg.Package, _ ports.Spinner) error {
	for dest, source := range p.Configs {
		var content string
		if p.Type == pkg.TypeConfig && p.ConfigDir != "" {
			data, err := i.fs.ReadFile(filepath.Join(p.ConfigDir, source))
			if err != nil {
				return fmt.Errorf("reading config %s: %w", source, err)
			}
			content = string(data)
		} else {
			content = source
		}
		if err := i.deployer.Deploy(ctx, content, dest, 0644); err != nil {
			return fmt.Errorf("deploying %s: %w", dest, err)
		}
	}

	if err := i.deployUserConfigs(ctx, p); err != nil {
		return err
	}

	if err := i.deployer.RunPostInstall(ctx, p.PostInstall); err != nil {
		i.logger.Warn("post-install: %s", err)
	}
	return nil
}

func (i *ConfigInstaller) Remove(ctx context.Context, p *pkg.Package, _ ports.Spinner) error {
	if err := i.deployer.RemoveConfigs(ctx, p.Configs); err != nil {
		i.logger.Warn("removing configs: %v", err)
	}

	user, err := deployer.InvokingUser()
	if err != nil {
		i.logger.Warn("cannot determine invoking user: %v", err)
	} else {
		i.deployer.RemoveUserConfigs(ctx, p.UserConfigs, user)
	}

	return i.deployer.RunPostRemove(ctx, p.PostRemove)
}

func (i *ConfigInstaller) Update(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	return i.Install(ctx, p, spinner)
}

func (i *ConfigInstaller) deployUserConfigs(ctx context.Context, p *pkg.Package) error {
	if len(p.UserConfigs) == 0 {
		return nil
	}
	user, err := deployer.InvokingUser()
	if err != nil {
		return fmt.Errorf("determine invoking user: %w", err)
	}
	for path, source := range p.UserConfigs {
		var content string
		if p.Type == pkg.TypeConfig && p.ConfigDir != "" {
			data, err := i.fs.ReadFile(filepath.Join(p.ConfigDir, source))
			if err != nil {
				return fmt.Errorf("reading user config %s: %w", source, err)
			}
			content = string(data)
		} else {
			content = source
		}
		if err := i.deployer.DeployUserConfig(ctx, content, path, user); err != nil {
			return fmt.Errorf("deploying user config %s: %w", path, err)
		}
	}
	return nil
}

var _ Installer = (*ConfigInstaller)(nil)
