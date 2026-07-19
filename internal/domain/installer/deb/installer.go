// Package deb implements installer.Installer for deb-type packages (deb
// archives downloaded from a URL and installed via apt-get).
//
// Install is a thin wrapper: Prepare + execApt + Finalize. Prepare
// and Finalize are exported so the batch runner can call them directly;
// Finalize is also used by Abort's cleanup path.
package deb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/download"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/installer/version"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// DownloadFunc downloads a file from a URL. Matches download.Download's
// signature; injectable so tests can exercise Prepare's real logic
// (including the .deb suffix handling below) without a real HTTP request.
type DownloadFunc func(ctx context.Context, fs ports.FileSystem, url, dest string, spinner ports.Spinner, sha256 string) error

// Installer installs and removes .deb packages.
type Installer struct {
	runner       ports.CommandRunner
	fs           ports.FileSystem
	ui           ports.UI
	sys          ports.System
	execApt      aptpty.AptExecFunc
	downloadFunc DownloadFunc
	tempDirs     map[string]string // package name → temp dir (batch mode)
}

// NewInstaller returns a new deb Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI, sys ports.System) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, sys: sys, execApt: aptpty.AptExec, downloadFunc: download.Download}
}

// Install downloads the .deb file(s) from p.URLs, installs them via apt-get,
// and runs the post-install script.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	args, err := i.Prepare(ctx, p, spinner)
	if err != nil {
		return err
	}
	if args.Skipped {
		return nil
	}
	installArgs := append([]string{"install", "-y"}, args.AptPkgs...)
	installArgs = append(installArgs, args.DebPaths...)
	if err := i.execApt(ctx, i.runner, installArgs, spinner); err != nil {
		return fmt.Errorf("install %s: %w", p.Name, err)
	}
	return i.Finalize(ctx, p, spinner)
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

// ---- batch support --------------------------------------------------------

// Prepare does per-package setup without running the main apt-get install.
// It handles version check, prerequisite installation, and .deb downloading.
// Returns BatchArgs with the .deb file paths for the batch apt-get install.
func (i *Installer) Prepare(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (installer.BatchArgs, error) {
	if err := installer.AssertType(p.Type, pkg.TypeDeb, "deb"); err != nil {
		return installer.BatchArgs{}, err
	}
	if len(p.URLs) == 0 {
		return installer.BatchArgs{}, fmt.Errorf("deb definition %q: no install url", p.Name)
	}

	if p.VersionCmd != "" || version.RepoFromPkg(p) != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return installer.BatchArgs{}, err
		}
		if !updated && !p.ForceInstall {
			ok, err := installer.CheckInstalled(ctx, i.runner, i.fs, i.sys, p)
			if err != nil {
				return installer.BatchArgs{}, err
			}
			if ok {
				return installer.BatchArgs{Skipped: true}, nil
			}
		}
	}

	if len(p.Packages) > 0 {
		spinner.SetDesc("installing prerequisites for " + p.Name)
		if err := i.execApt(ctx, i.runner, append([]string{"install", "-y"}, p.Packages...), spinner); err != nil {
			return installer.BatchArgs{}, fmt.Errorf("install prerequisites %s: %w", p.Name, err)
		}
	}

	tmpDir, err := installer.MkdirTemp(i.fs)
	if err != nil {
		return installer.BatchArgs{}, err
	}

	tmpPaths := make([]string, 0, len(p.URLs))
	for idx, raw := range p.URLs {
		url := download.ExpandURL(raw, p.Version)
		tmpPath := filepath.Join(tmpDir, download.FilenameFromURL(url))
		if !strings.HasSuffix(strings.ToLower(tmpPath), ".deb") {
			tmpPath += ".deb"
		}
		tmpPaths = append(tmpPaths, tmpPath)

		spinner.SetDesc("downloading " + p.Name)
		sha256 := ""
		if idx < len(p.SHA256s) {
			sha256 = p.SHA256s[idx]
		}
		if err := i.downloadFunc(ctx, i.fs, url, tmpPath, spinner, sha256); err != nil {
			_ = i.fs.RemoveAll(tmpDir)
			return installer.BatchArgs{}, fmt.Errorf("download %s: %w", p.Name, err)
		}
	}

	if i.tempDirs == nil {
		i.tempDirs = make(map[string]string)
	}
	i.tempDirs[p.Name] = tmpDir

	return installer.BatchArgs{DebPaths: tmpPaths}, nil
}

// Finalize runs post-install work after the batch apt-get install:
// post-install scripts and temp dir cleanup.
func (i *Installer) Finalize(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	err := installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall)
	i.cleanupTempDir(p)
	return err
}

// Abort releases the temp dir created by Prepare without running
// postinstall, for when the batch's shared apt-get call failed and this
// package was never actually installed.
func (i *Installer) Abort(p *pkg.Package) {
	i.cleanupTempDir(p)
}

func (i *Installer) cleanupTempDir(p *pkg.Package) {
	if i.tempDirs == nil {
		return
	}
	if tmpDir, ok := i.tempDirs[p.Name]; ok {
		delete(i.tempDirs, p.Name)
		_ = i.fs.RemoveAll(tmpDir)
	}
}
