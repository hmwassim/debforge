package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func saveState(state *StateManager, st *State, context string) error {
	if err := state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", context, err)
	}
	return nil
}

func checkInstalled(ctx context.Context, state *StateManager, st *State, name string, runner ports.CommandRunner, fs ports.FileSystem, p *pkg.Package, spinner ports.Spinner) (cleanedUp bool, err error) {
	if !state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return false, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	if !allPackagesInstalled(ctx, runner, fs, p) {
		state.Remove(st, name)
		spinner.SetDesc(name + " not installed")
		return true, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	return false, nil
}

// allPackagesInstalled delegates to installer.CheckInstalled. The function
// is kept as an internal indirection so callers (checkInstalled, processOne)
// share a single call pattern regardless of the installer package API.
func allPackagesInstalled(ctx context.Context, runner ports.CommandRunner, fs ports.FileSystem, p *pkg.Package) bool {
	return installer.CheckInstalled(ctx, runner, fs, p)
}
