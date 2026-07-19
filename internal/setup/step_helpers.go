package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

// ConfigFile represents a config file to write during setup.
type ConfigFile struct {
	Path    string
	Content string
}

// checkConfigFiles runs the standard 3-way-merge check for a list of config
// files. Returns StatusSatisfied when all files are in the expected state.
func checkConfigFiles(cx *Context, cfgs []ConfigFile) CheckResult {
	for _, cfg := range cfgs {
		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], false)
		exists, err := cx.Fsys.Exists(cfg.Path)
		if err != nil {
			return CheckResult{Status: StatusError, Summary: fmt.Sprintf("check %s: %s", cfg.Path, err)}
		}
		switch {
		case action == installer.ConfigWrite && !exists:
			return CheckResult{Status: StatusMissing, Summary: fmt.Sprintf("%s does not exist", cfg.Path)}
		case action == installer.ConfigWrite && exists:
			continue
		case action == installer.ConfigSkip:
			return CheckResult{Status: StatusDrifted, Summary: fmt.Sprintf("%s modified by user", cfg.Path)}
		case action == installer.ConfigConflict:
			return CheckResult{Status: StatusConflict, Summary: fmt.Sprintf("%s: local changes conflict with new defaults", cfg.Path)}
		}
	}
	return CheckResult{Status: StatusSatisfied}
}

// processConfigFiles runs the standard 3-way-merge apply for a list of config
// files, creating parent directories, writing or skipping files, and handling
// conflicts by writing a .debforge-new sidecar.
func processConfigFiles(cx *Context, cfgs []ConfigFile, result CheckResult) error {
	for _, cfg := range cfgs {
		force := cx.Force
		if result.Status == StatusDrifted {
			force = false
		}

		action := installer.DecideConfigAction(cx.Fsys, cfg.Path, cfg.Content, cx.ConfigHashes[cfg.Path], force)

		switch action {
		case installer.ConfigWrite:
			hash, err := installer.WriteConfigFile(cx.Fsys, cfg.Path, cfg.Content)
			if err != nil {
				return err
			}
			cx.ConfigHashes[cfg.Path] = hash

		case installer.ConfigSkip:
			diskData, err := cx.Fsys.ReadFile(cfg.Path)
			if err == nil && diskData != nil {
				cx.ConfigHashes[cfg.Path] = textutil.Sha256Hex(diskData)
			}

		case installer.ConfigConflict:
			if err := installer.WriteConfigSidecar(cx.Fsys, cfg.Path, cfg.Content); err != nil {
				return fmt.Errorf("write sidecar: %w", err)
			}
			cx.UI.Warn("%s has local changes; new version saved as %s", cfg.Path, cfg.Path+".debforge-new")
		}
	}
	return nil
}

// checkStepPackages is a shared Check helper for steps that install packages.
// It wraps allInstalled with consistent error handling.
func checkStepPackages(ctx context.Context, cx *Context, pkgs []string, summary string) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, pkgs)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: summary}
	}
	return CheckResult{Status: StatusSatisfied}
}

func allInstalled(ctx context.Context, runner ports.CommandRunner, names []string) (bool, error) {
	if len(names) == 0 {
		return true, nil
	}
	args := []string{"-W", "-f=${db:Status-Status}\n"}
	args = append(args, names...)
	out, _, err := runner.Run(ctx, "dpkg-query", args...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "installed" {
			return false, nil
		}
	}
	return true, nil
}
