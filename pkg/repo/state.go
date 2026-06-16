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

func LoadState() *PackagesState {
	s := &PackagesState{Version: 1, Packages: map[string]PkgEntry{}}
	store := state.New("packages")
	if err := store.Load(s); err != nil {
		return &PackagesState{Version: 1, Packages: map[string]PkgEntry{}}
	}
	if s.Packages == nil {
		s.Packages = map[string]PkgEntry{}
	}
	return s
}

func saveState(s *PackagesState) error {
	store := state.New("packages")
	return store.Save(s)
}
