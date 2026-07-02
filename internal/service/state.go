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

// PkgEntry is the persisted metadata for a single installed package.
type PkgEntry struct {
	Type         string            `json:"type"`
	Variant      string            `json:"variant,omitempty"`
	Version      string            `json:"version,omitempty"`
	ConfigHashes map[string]string `json:"config_hashes,omitempty"`
}

// State is the persisted state of all installed packages.
type State struct {
	Packages map[string]PkgEntry `json:"packages"`
}

// StateManager provides thread-safe load/save access to the installation
// state, backed by a generic JSON store.
type StateManager struct {
	store *store.Store[State]
	mu    sync.Mutex
}

// NewStateManager returns a StateManager backed by the given store.
func NewStateManager(st *store.Store[State]) *StateManager {
	return &StateManager{store: st}
}

// Load reads the persisted state, returning an empty State when the file
// does not exist.
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
	if st.Packages == nil {
		st.Packages = make(map[string]PkgEntry)
	}
	return st, nil
}

// Save persists the state to the JSON store.
func (m *StateManager) Save(st *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Save(st)
}

// IsInstalled reports whether name is recorded in the state.
func (m *StateManager) IsInstalled(st *State, name string) bool {
	_, ok := st.Packages[name]
	return ok
}

// Entry returns the PkgEntry for name and whether it exists.
func (m *StateManager) Entry(st *State, name string) (PkgEntry, bool) {
	e, ok := st.Packages[name]
	return e, ok
}

// ListPackages returns all package names in the state.
func (m *StateManager) ListPackages(st *State) []string {
	names := make([]string, 0, len(st.Packages))
	for n := range st.Packages {
		names = append(names, n)
	}
	return names
}

// Add records name in the state with the given entry.
func (m *StateManager) Add(st *State, name string, entry PkgEntry) {
	st.Packages[name] = entry
}

// Remove deletes name from the state.
func (m *StateManager) Remove(st *State, name string) {
	delete(st.Packages, name)
}

func saveState(state *StateManager, st *State, label string) error {
	if err := state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", label, err)
	}
	return nil
}

func checkInstalled(ctx context.Context, state *StateManager, st *State, name string, runner ports.CommandRunner, fs ports.FileSystem, sys ports.System, p *pkg.Package, spinner ports.Spinner) (cleanedUp bool, err error) {
	if !state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return false, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	ok, err := installer.CheckInstalled(ctx, runner, fs, sys, p)
	if err != nil {
		return false, err
	}
	if !ok {
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
