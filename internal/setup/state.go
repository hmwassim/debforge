package setup

import (
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/ports"
)

// State records per-config-file hashes so the setup runner can detect
// user modifications between runs.
type State struct {
	ConfigHashes map[string]string `json:"config_hashes,omitempty"`
}

// LoadState reads the setup state from disk, returning an empty State
// when the file does not exist.
func LoadState(fsys ports.FileSystem, path string) (*State, error) {
	s := store.NewStore[State](fsys, path)
	st, err := s.Load()
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &State{ConfigHashes: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("load setup state: %w", err)
	}
	if st.ConfigHashes == nil {
		st.ConfigHashes = make(map[string]string)
	}
	return st, nil
}

// SaveState persists the setup state to disk.
func SaveState(fsys ports.FileSystem, path string, st *State) error {
	s := store.NewStore[State](fsys, path)
	return s.Save(st)
}
