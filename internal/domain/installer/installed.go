// Package installer provides shared types and helpers for all package-type
// installers (apt, deb, source, config), including the Installer interface,
// config file management, script execution, and installation-state checks.
package installer

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// CheckInstalled verifies that p is installed on the system.
//   - apt: all system package names in p.Packages (or PrimarySystemPackage()
//     when Packages is empty) are dpkg-installed.
//   - deb: PrimarySystemPackage() is dpkg-installed.
//   - config: every file in p.Configs exists on disk.
//   - source: falls back to package metadata (state.json); no universal
//     system check exists, so returns true unconditionally.
//
// The caller is responsible for reconciling this result with state.json.
func CheckInstalled(ctx context.Context, runner ports.CommandRunner, fs ports.FileSystem, p *pkg.Package) bool {
	switch p.Type {
	case pkg.TypeApt:
		name := p.PrimarySystemPackage()
		if p.Apt != nil && p.Apt.Variant != "" {
			if v, ok := p.Apt.Variants[p.Apt.Variant]; ok && len(v) > 0 {
				name = v[0]
			}
		}
		return dpkg.IsInstalled(ctx, runner, name)
	case pkg.TypeDeb:
		name := p.PrimarySystemPackage()
		if dpkg.IsInstalled(ctx, runner, name) {
			return true
		}
		return dpkg.IsInstalled(ctx, runner, p.Name)
	case pkg.TypeConfig:
		return configsInstalled(fs, p)
	default: // source
		return true
	}
}

// configsInstalled checks that every system config file in p.Configs
// and every user config file (with ~ expansion) in p.UserConfigs exists.
func configsInstalled(fs ports.FileSystem, p *pkg.Package) bool {
	for path := range p.Configs {
		ok, err := fs.Exists(path)
		if err != nil || !ok {
			return false
		}
	}
	homeDir, homeErr := UserHomeDir()
	for path := range p.UserConfigs {
		if homeErr != nil {
			return false
		}
		absPath := ExpandHome(path, homeDir)
		ok, err := fs.Exists(absPath)
		if err != nil || !ok {
			return false
		}
	}
	return true
}
