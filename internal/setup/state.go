package setup

import (
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/ports"
)

type State struct {
	ConfigHashes map[string]string `json:"config_hashes,omitempty"`
}

func LoadState(fsys ports.FileSystem, path string) (*State, error) {
	s := store.NewStore[State](fsys, path)
	st, err := s.Load()
	if err != nil {
		return &State{ConfigHashes: make(map[string]string)}, nil
	}
	if st.ConfigHashes == nil {
		st.ConfigHashes = make(map[string]string)
	}
	return st, nil
}

func SaveState(fsys ports.FileSystem, path string, st *State) error {
	s := store.NewStore[State](fsys, path)
	return s.Save(st)
}
