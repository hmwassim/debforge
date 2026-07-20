package service

import (
	"errors"
	"sync"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

// PkgEntry is the persisted metadata for a single installed package.
type PkgEntry struct {
	Type         string            `json:"type"`
	Variant      string            `json:"variant,omitempty"`
	Version      string            `json:"version,omitempty"`
	ConfigHashes map[string]string `json:"config_hashes,omitempty"`
}

// State is the persisted state of all installed packages. All map access
// is serialised by mu so concurrent goroutines may safely call
// StateManager methods that share the same *State.
type State struct {
	mu       sync.RWMutex
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
	st.mu.RLock()
	_, ok := st.Packages[name]
	st.mu.RUnlock()
	return ok
}

// Entry returns the PkgEntry for name and whether it exists.
func (m *StateManager) Entry(st *State, name string) (PkgEntry, bool) {
	st.mu.RLock()
	e, ok := st.Packages[name]
	st.mu.RUnlock()
	return e, ok
}

// ListPackages returns all package names in the state.
func (m *StateManager) ListPackages(st *State) []string {
	st.mu.RLock()
	names := make([]string, 0, len(st.Packages))
	for n := range st.Packages {
		names = append(names, n)
	}
	st.mu.RUnlock()
	return names
}

// Add records name in the state with the given entry.
func (m *StateManager) Add(st *State, name string, entry PkgEntry) {
	st.mu.Lock()
	st.Packages[name] = entry
	st.mu.Unlock()
}

// Remove deletes name from the state.
func (m *StateManager) Remove(st *State, name string) {
	st.mu.Lock()
	delete(st.Packages, name)
	st.mu.Unlock()
}

// lookupVariant returns the variant saved in state for name, or "".
func lookupVariant(st *State, name string) string {
	st.mu.RLock()
	entry, ok := st.Packages[name]
	st.mu.RUnlock()
	if ok {
		return entry.Variant
	}
	return ""
}

// has reports whether name is present in the state (read-locked).
func has(st *State, name string) bool {
	st.mu.RLock()
	_, ok := st.Packages[name]
	st.mu.RUnlock()
	return ok
}

// newPkgEntry builds a PkgEntry from a resolved package.
func newPkgEntry(p *pkg.Package) PkgEntry {
	e := PkgEntry{
		Type:         string(p.Type),
		Version:      p.Version,
		ConfigHashes: p.ConfigHashes,
	}
	if p.Apt != nil {
		e.Variant = p.Apt.Variant
	}
	return e
}

// applyVariant clones p and applies the variant recorded in st so that
// subsequent install/remove/update operations target the right system
// packages. Returns p unchanged when no variant is stored.
func applyVariant(p *pkg.Package, st *State, name string) *pkg.Package {
	if p.Apt != nil {
		if v := lookupVariant(st, name); v != "" {
			p = p.Clone()
			p.Apt.Variant = v
		}
	}
	return p
}
