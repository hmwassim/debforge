package service

import "sync"

type PkgEntry struct {
	Type    string `json:"type"`
	Variant string `json:"variant,omitempty"`
	Version string `json:"version,omitempty"`
}

type State struct {
	Version  int                 `json:"version"`
	Packages map[string]PkgEntry `json:"packages"`
}

type StateStore interface {
	Load() (*State, error)
	Save(*State) error
}

type StateManager struct {
	store StateStore
	mu    sync.Mutex
}

func NewStateManager(store StateStore) *StateManager {
	return &StateManager{store: store}
}

func (m *StateManager) Load() (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Load()
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

func (m *StateManager) InstalledMap(st *State) map[string]bool {
	installed := make(map[string]bool, len(st.Packages))
	for n := range st.Packages {
		installed[n] = true
	}
	return installed
}

func (m *StateManager) Add(st *State, name string, entry PkgEntry) {
	st.Packages[name] = entry
}

func (m *StateManager) Remove(st *State, name string) {
	delete(st.Packages, name)
}
