package service

import (
	"errors"
	"sync"

	"github.com/hmwassim/debforge/internal/adapters/store"
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
