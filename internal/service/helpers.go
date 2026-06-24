package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func saveState(state *StateManager, st *State, context string) error {
	if err := state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", context, err)
	}
	return nil
}

func checkInstalled(ctx context.Context, state *StateManager, st *State, name string, runner ports.CommandRunner, pkgName string, typ pkg.Type, spinner ports.Spinner) error {
	if !state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	if (typ == pkg.TypeDeb || typ == pkg.TypeApt) && runner != nil && pkgName != "" {
		if !systemPackageInstalled(ctx, runner, pkgName) {
			spinner.SetDesc(name + " not installed")
			return fmt.Errorf("%w: %s", ErrNotInstalled, name)
		}
	}
	return nil
}

func systemPackageInstalled(ctx context.Context, runner ports.CommandRunner, pkgName string) bool {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f", "${Status}", pkgName)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "install ok installed")
}
