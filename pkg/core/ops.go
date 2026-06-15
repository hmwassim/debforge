package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

func Repair(log *text.Logger) error {
	log.Info("Repairing core system...")

	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	var errs []error

	if err := ensureSourcesList(); err != nil {
		errs = append(errs, fmt.Errorf("sources.list: %w", err))
	}
	if err := enablei386(); err != nil {
		errs = append(errs, fmt.Errorf("i386: %w", err))
	}

	if len(errs) == 0 {
		if err := executil.RunWithSpinner(exec.Command("apt", "update"), "Updating package lists..."); err != nil {
			errs = append(errs, fmt.Errorf("apt update: %w", err))
		}
	}

	for _, g := range groups {
		if err := packages.AptInstall(g.packages, g.backport, "Installing "+g.name+"..."); err != nil {
			errs = append(errs, fmt.Errorf("installing %s: %w", g.name, err))
			continue
		}

		for _, cf := range g.configs {
			if err := packages.DeployConfig(cf.dest, cf.content, cf.mode); err != nil {
				errs = append(errs, fmt.Errorf("deploying %s: %w", cf.dest, err))
			}
		}

		for _, svc := range g.services {
			if err := packages.EnableService(svc); err != nil {
				log.Warn("Failed to start %s: %s", svc, err)
			}
		}

		if g.postInstall != nil {
			if err := g.postInstall(log); err != nil {
				errs = append(errs, fmt.Errorf("post-install %s: %w", g.name, err))
			}
		}
	}

	if err := ensureResolvSymlink(); err != nil {
		errs = append(errs, fmt.Errorf("resolv.conf symlink: %w", err))
	}

	if len(errs) > 0 {
		for _, err := range errs {
			log.Error("%s", err)
		}
		return fmt.Errorf("repair completed with %d error(s)", len(errs))
	}

	log.Success("Core repair complete")
	return nil
}

func Update(log *text.Logger) error {
	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	if err := executil.RunWithSpinner(exec.Command("apt", "update"), "Updating package lists..."); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	var defaultPkgs, backportPkgs []string
	for _, g := range groups {
		if g.backport {
			backportPkgs = append(backportPkgs, g.packages...)
		} else {
			defaultPkgs = append(defaultPkgs, g.packages...)
		}
	}

	if err := packages.AptInstall(defaultPkgs, false, "Upgrading core packages..."); err != nil {
		return fmt.Errorf("upgrading core: %w", err)
	}
	if err := packages.AptInstall(backportPkgs, true, "Upgrading backports..."); err != nil {
		return fmt.Errorf("upgrading backports: %w", err)
	}

	log.Success("Core packages up to date")
	return nil
}

func List(log *text.Logger) {
	log.Info("Core packages:")

	var allPkgs []string
	pkgToGroup := map[string]string{}
	for _, g := range groups {
		for _, pkg := range g.packages {
			allPkgs = append(allPkgs, pkg)
			pkgToGroup[pkg] = g.name
		}
	}

	installed, err := packages.CheckInstalled(allPkgs)
	if err != nil {
		log.Warn("Could not query package status: %s", err)
		return
	}

	type groupStatus struct {
		missing []string
	}
	statuses := map[string]*groupStatus{}
	for _, g := range groups {
		statuses[g.name] = &groupStatus{}
	}
	for pkg, gname := range pkgToGroup {
		if !installed[pkg] {
			statuses[gname].missing = append(statuses[gname].missing, pkg)
		}
	}
	for _, g := range groups {
		s := statuses[g.name]
		if len(s.missing) == 0 {
			log.Success("  %s — installed", g.name)
		} else {
			log.Warn("  %s — missing: %s", g.name, strings.Join(s.missing, ", "))
		}
	}
}

func ensureSourcesList() error {
	const path = "/etc/apt/sources.list"
	data, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(data), "trixie") {
		return nil
	}
	return os.WriteFile(path, []byte(sourcesList), 0644)
}

func enablei386() error {
	cmd := exec.Command("dpkg", "--print-foreign-architectures")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := executil.Run(cmd)
	if err == nil && strings.Contains(stdout.String(), "i386") {
		return nil
	}
	return executil.Run(exec.Command("dpkg", "--add-architecture", "i386"))
}

func installFlathub(log *text.Logger) error {
	return executil.RunWithSpinner(exec.Command("flatpak", "remote-add", "--if-not-exists", "flathub", "https://flathub.org/repo/flathub.flatpakrepo"), "Adding Flathub remote...")
}

func ensureResolvSymlink() error {
	const target = "/run/systemd/resolve/stub-resolv.conf"
	const link = "/etc/resolv.conf"
	current, err := os.Readlink(link)
	if err == nil && current == target {
		return nil
	}
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, link)
}
