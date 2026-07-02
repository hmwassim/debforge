// Package config implements installer.Installer for config-type packages
// (static configuration files and user-level config files).
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer installs and removes config files defined by a Package.
type Installer struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
	ui     ports.UI
	sys    ports.System
}

// NewInstaller returns a new config Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI, sys ports.System) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, sys: sys}
}

// Install writes the config files and user config files defined by p,
// skipping install when the version hash matches (unless ForceInstall).
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeConfig {
		return fmt.Errorf("config installer called for type %s", p.Type)
	}

	newHash := computeConfigHash(p)
	if !p.ForceInstall && p.Version != "" && p.Version == newHash {
		return nil
	}

	if err := i.writeConfigs(p, spinner); err != nil {
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
	hashMap(h, p.Configs)
	hashMap(h, p.UserConfigs)
	return hex.EncodeToString(h.Sum(nil))
}

func hashMap(h io.Writer, m map[string]string) {
	paths := make([]string, 0, len(m))
	for k := range m {
		paths = append(paths, k)
	}
	slices.Sort(paths)
	for _, path := range paths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write([]byte(m[path]))
		h.Write([]byte{0})
	}
}

// Remove deletes the user configs, remove configs, and system configs
// defined by p, skipping modified files unless ForceInstall is set.
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
		homeDir, err := installer.UserHomeDir(i.sys)
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		absPath := installer.ExpandHome(path, homeDir)

		action := installer.DecideConfigAction(i.fs, absPath, content, p.ConfigHashes[absPath], p.ForceInstall)
		if action == installer.ConfigSkip || action == installer.ConfigConflict {
			spinner.SetDesc("skipping modified user config " + path)
			continue
		}

		if err := i.fs.RemoveAll(absPath); err != nil {
			return fmt.Errorf("remove user config %s: %w", path, err)
		}
	}

	for path := range p.RemoveConfigs {
		spinner.SetDesc("removing config " + path)
		absPath := path
		if installer.HasHomePrefix(path) {
			homeDir, err := installer.UserHomeDir(i.sys)
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
		content := p.Configs[path]
		action := installer.DecideConfigAction(i.fs, path, content, p.ConfigHashes[path], p.ForceInstall)
		if action == installer.ConfigSkip || action == installer.ConfigConflict {
			spinner.SetDesc("skipping modified config " + path)
			continue
		}
		if err := i.fs.RemoveAll(path); err != nil {
			return fmt.Errorf("remove config %s: %w", path, err)
		}
	}

	return nil
}

func (i *Installer) writeConfigs(p *pkg.Package, spinner ports.Spinner) error {
	hashes := p.ConfigHashes
	if hashes == nil {
		hashes = make(map[string]string)
	}

	updated, err := installer.WriteConfigsWithHashes(i.fs, spinner, p, hashes)
	if err != nil {
		return err
	}
	hashes = updated

	updated, err = installer.WriteUserConfigsWithHashes(i.fs, i.sys, spinner, p, hashes)
	if err != nil {
		return err
	}
	p.ConfigHashes = updated
	return nil
}
