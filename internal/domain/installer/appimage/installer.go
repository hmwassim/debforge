// Package appimage implements installer.Installer for appimage-type packages
// (AppImage binaries downloaded from a URL, made executable, and installed
// into /usr/local/bin, optionally with a desktop entry and embedded icon).
package appimage

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

// binDir is where AppImage binaries are installed. It is on PATH so the
// installed command is directly invocable.
const binDir = "/usr/local/bin"

// DownloadFunc downloads a file from a URL. Matches download.Download's
// signature; injectable so tests can exercise Install's real logic without a
// real HTTP request.
type DownloadFunc func(ctx context.Context, fs ports.FileSystem, url, dest string, spinner ports.Spinner, sha256 string) error

// Installer installs and removes AppImage packages.
type Installer struct {
	runner       ports.CommandRunner
	fs           ports.FileSystem
	ui           ports.UI
	sys          ports.System
	execApt      aptpty.AptExecFunc
	downloadFunc DownloadFunc
}

// NewInstaller returns a new appimage Installer.
func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem, ui ports.UI, sys ports.System) *Installer {
	return &Installer{runner: runner, fs: fs, ui: ui, sys: sys, execApt: aptpty.AptExec, downloadFunc: download.Download}
}

// Install downloads the AppImage, installs it into binDir, extracts and
// installs the embedded icon, writes the desktop entry, and runs the
// post-install script.
func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeAppImage, "appimage"); err != nil {
		return err
	}
	if len(p.URLs) == 0 {
		return fmt.Errorf("appimage definition %q: no install url", p.Name)
	}

	if p.VersionCmd != "" || version.RepoFromPkg(p) != "" {
		updated, err := i.checkVersion(ctx, p, spinner)
		if err != nil {
			return err
		}
		if !updated && !p.ForceInstall {
			ok, err := installer.CheckInstalled(ctx, i.runner, i.fs, i.sys, p)
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

	return installer.WithTempDir(i.fs, p.Name, func(tmpDir string) error {
		url := download.ExpandURL(p.URLs[0], p.Version)
		archive := filepath.Join(tmpDir, download.FilenameFromURL(url))
		sha256 := ""
		if len(p.SHA256s) > 0 {
			sha256 = p.SHA256s[0]
		}

		spinner.SetDesc("downloading " + p.Name)
		if err := i.downloadFunc(ctx, i.fs, url, archive, spinner, sha256); err != nil {
			return fmt.Errorf("download %s: %w", p.Name, err)
		}

		binName := i.binName(p)
		binPath := filepath.Join(binDir, binName)

		spinner.SetDesc("installing " + p.Name)
		if _, _, err := i.runner.Run(ctx, "install", "-Dm755", archive, binPath); err != nil {
			return fmt.Errorf("install binary %s: %w", p.Name, err)
		}

		if p.AppImg.Desktop != nil {
			if err := i.installDesktop(ctx, p, tmpDir, archive, spinner); err != nil {
				return err
			}
		}

		return installer.RunPostInstall(ctx, i.runner, spinner, p.Name, p.PostInstall)
	})
}

// Remove deletes the installed binary, desktop entry, and icons, removes
// apt packages listed in p.Remove, and runs the post-remove script.
func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if err := installer.AssertType(p.Type, pkg.TypeAppImage, "appimage"); err != nil {
		return err
	}

	binName := i.binName(p)
	paths := []string{filepath.Join(binDir, binName)}

	if p.AppImg.Desktop != nil {
		id := p.AppImg.Desktop.ID
		if id == "" {
			id = binName
		}
		paths = append(paths, filepath.Join("/usr/share/applications", id+".desktop"))
		if icon := p.AppImg.Desktop.Icon; icon != "" {
			paths = append(paths,
				filepath.Join("/usr/share/icons/hicolor/scalable/apps", icon+".svg"),
				filepath.Join("/usr/share/icons/hicolor/128x128/apps", icon+".png"),
			)
		}
	}

	spinner.SetDesc("removing " + p.Name + "...")
	for _, path := range paths {
		if err := installer.ValidateRemovablePath(path); err != nil {
			return err
		}
		if err := i.fs.RemoveAll(path); err != nil {
			return err
		}
	}

	if len(p.Remove) > 0 {
		if err := i.execApt(ctx, i.runner, append([]string{"remove", "-y"}, p.Remove...), spinner); err != nil {
			return err
		}
	}

	return installer.RunPostRemove(ctx, i.runner, spinner, p.Name, p.PostRemove)
}

// installDesktop extracts the embedded icon from the AppImage and writes a
// .desktop entry pointing at the installed binary.
func (i *Installer) installDesktop(ctx context.Context, p *pkg.Package, tmpDir, archive string, spinner ports.Spinner) error {
	d := p.AppImg.Desktop

	if d.Icon != "" {
		spinner.SetDesc("extracting icon for " + p.Name)
		if _, _, err := i.runner.RunWithOptions(ctx, ports.RunOptions{Dir: tmpDir}, archive, "--appimage-extract"); err != nil {
			return fmt.Errorf("extract appimage %s: %w", p.Name, err)
		}
		iconPath, err := i.findIcon(filepath.Join(tmpDir, "squashfs-root"), d.Icon)
		if err != nil {
			return err
		}
		if iconPath != "" {
			if err := i.installIcon(ctx, iconPath, d.Icon); err != nil {
				return err
			}
		}
	}

	id := d.ID
	if id == "" {
		id = i.binName(p)
	}
	desktopPath := filepath.Join("/usr/share/applications", id+".desktop")

	content := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + d.Name + "\n"
	if d.Comment != "" {
		content += "Comment=" + d.Comment + "\n"
	}
	content += "Exec=" + filepath.Join(binDir, i.binName(p)) + "\n"
	if d.Icon != "" {
		content += "Icon=" + d.Icon + "\n"
	}
	content += "Categories=" + d.Categories + "\n"
	if d.Terminal {
		content += "Terminal=true\n"
	}

	spinner.SetDesc("writing desktop entry for " + p.Name)
	if err := i.fs.WriteFile(desktopPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write desktop entry %s: %w", p.Name, err)
	}
	return nil
}

// findIcon searches the extracted AppImage tree for an icon file whose base
// name is <name>.svg or <name>.png, returning the first match ("" if none).
func (i *Installer) findIcon(rootDir, iconName string) (string, error) {
	found := ""
	err := i.fs.Walk(rootDir, func(path string, info ports.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info == nil || info.IsDir() || found != "" {
			return nil
		}
		base := info.Name()
		if base == iconName+".svg" || base == iconName+".png" {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search icon %s: %w", iconName, err)
	}
	return found, nil
}

// installIcon copies the extracted icon into the hicolor theme: scalable for
// svg, 128x128 for png.
func (i *Installer) installIcon(ctx context.Context, src, iconName string) error {
	ext := filepath.Ext(src)
	dest := filepath.Join("/usr/share/icons/hicolor/128x128/apps", iconName+".png")
	if ext == ".svg" {
		dest = filepath.Join("/usr/share/icons/hicolor/scalable/apps", iconName+".svg")
	}
	if _, _, err := i.runner.Run(ctx, "install", "-Dm644", src, dest); err != nil {
		return fmt.Errorf("install icon %s: %w", iconName, err)
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

func (i *Installer) binName(p *pkg.Package) string {
	if p.AppImg != nil && p.AppImg.Bin != "" {
		return p.AppImg.Bin
	}
	return p.Name
}
