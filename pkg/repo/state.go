package repo

import (
	"github.com/hmwassim/debforge/pkg/state"
)

type PkgEntry struct {
	Type string `json:"type"`
}

type PackagesState struct {
	Version  int                 `json:"version"`
	Packages map[string]PkgEntry `json:"packages"`
}

func LoadState() (*PackagesState, error) {
	s := &PackagesState{Version: 1, Packages: map[string]PkgEntry{}}
	store := state.New("packages")
	if err := store.Load(s); err != nil {
		return nil, err
	}
	if s.Packages == nil {
		s.Packages = map[string]PkgEntry{}
	}
	return s, nil
}

func saveState(s *PackagesState) error {
	store := state.New("packages")
	return store.Save(s)
}
