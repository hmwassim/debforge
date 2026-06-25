package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

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

	newHash := computeConfigHash(p)
	if !p.ForceInstall && p.Version != "" && p.Version == newHash {
		return nil
	}

	if err := i.writeConfigs(ctx, p, spinner); err != nil {
		return err
	}

	if err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall); err != nil {
		return err
	}

	p.Version = newHash
	return nil
}

func computeConfigHash(p *pkg.Package) string {
	h := sha256.New()
	paths := make([]string, 0, len(p.Configs))
	for path := range p.Configs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write([]byte(p.Configs[path]))
		h.Write([]byte{0})
	}
	userPaths := make([]string, 0, len(p.UserConfigs))
	for path := range p.UserConfigs {
		userPaths = append(userPaths, path)
	}
	sort.Strings(userPaths)
	for _, path := range userPaths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write([]byte(p.UserConfigs[path]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeConfig {
		return fmt.Errorf("config installer called for type %s", p.Type)
	}

	if err := installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove); err != nil {
		return err
	}

	// Remove user-owned paths first (no root needed), then system-level
	// paths, so that a permission error on a system config doesn't orphan
	// user configs.

	for path, content := range p.UserConfigs {
		spinner.SetDesc("removing user config " + path)
		homeDir, err := installer.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		absPath := installer.ExpandHome(path, homeDir)

		if !p.ForceInstall {
			if ok, _ := i.fs.Exists(absPath); ok {
				existing, err := i.fs.ReadFile(absPath)
				if err == nil && string(existing) != content {
					spinner.SetDesc("skipping modified user config " + path)
					continue
				}
			}
		}

		if err := i.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("remove user config %s: %w", path, err)
		}
	}

	for path := range p.RemoveConfigs {
		spinner.SetDesc("removing config " + path)
		absPath := path
		if installer.HasHomePrefix(path) {
			homeDir, err := installer.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			absPath = installer.ExpandHome(path, homeDir)
		}
		if err := i.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("remove config %s: %w", path, err)
		}
	}

	for path := range p.Configs {
		spinner.SetDesc("removing config " + path)
		if err := i.fs.RemoveAll(path); err != nil {
			return fmt.Errorf("remove config %s: %w", path, err)
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
