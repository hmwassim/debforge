package service

import (
	"fmt"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

func saveState(state *StateManager, st *State, context string) error {
	if err := state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", context, err)
	}
	return nil
}

func checkInstalled(state *StateManager, st *State, name string, spinner ports.Spinner) error {
	if !state.IsInstalled(st, name) {
		spinner.SetDesc(textutil.UcFirst(name + " not installed"))
		return fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	return nil
}
