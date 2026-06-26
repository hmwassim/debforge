package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type PkgEntry struct {
	Type    string `json:"type"`
	Variant string `json:"variant,omitempty"`
	Version string `json:"version,omitempty"`
}

type State struct {
	Packages map[string]PkgEntry `json:"packages"`
}

type StateManager struct {
	store *store.Store[State]
	mu    sync.Mutex
}

func NewStateManager(st *store.Store[State]) *StateManager {
	return &StateManager{store: st}
}

func (m *StateManager) Load() (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, err := m.store.Load()
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &State{Packages: make(map[string]PkgEntry)}, nil
		}
		return nil, err
	}
	return st, nil
}

func (m *StateManager) Save(st *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Save(st)
}

func (m *StateManager) IsInstalled(st *State, name string) bool {
	_, ok := st.Packages[name]
	return ok
}

func (m *StateManager) Entry(st *State, name string) (PkgEntry, bool) {
	e, ok := st.Packages[name]
	return e, ok
}

func (m *StateManager) ListPackages(st *State) []string {
	names := make([]string, 0, len(st.Packages))
	for n := range st.Packages {
		names = append(names, n)
	}
	return names
}

func (m *StateManager) Add(st *State, name string, entry PkgEntry) {
	st.Packages[name] = entry
}

func (m *StateManager) Remove(st *State, name string) {
	delete(st.Packages, name)
}

func saveState(state *StateManager, st *State, label string) error {
	if err := state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", label, err)
	}
	return nil
}

func checkInstalled(ctx context.Context, state *StateManager, st *State, name string, runner ports.CommandRunner, fs ports.FileSystem, p *pkg.Package, spinner ports.Spinner) (cleanedUp bool, err error) {
	if !state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return false, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	if !installer.CheckInstalled(ctx, runner, fs, p) {
		state.Remove(st, name)
		spinner.SetDesc(name + " not installed")
		return true, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	return false, nil
}

// applyVariant clones p and applies the variant recorded in st so that
// subsequent install/remove/update operations target the right system
// packages. Returns p unchanged when no variant is stored.
func applyVariant(p *pkg.Package, st *State, name string) *pkg.Package {
	if entry, ok := st.Packages[name]; ok && p.Apt != nil && entry.Variant != "" {
		p = p.Clone()
		p.Apt.Variant = entry.Variant
	}
	return p
}
