package state

import (
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/statestore"
	"github.com/hmwassim/debforge/internal/ports"
)

type PkgEntry struct {
	Type    string `json:"type"`
	Variant string `json:"variant,omitempty"`
	Version string `json:"version,omitempty"`
}

type PackagesState struct {
	statestore.Versioned
	Packages map[string]PkgEntry `json:"packages"`
}

type Service struct {
	store     *statestore.Store
	fs        ports.FileSystem
	statePath string
}

func NewService(fs ports.FileSystem, statesDir string) *Service {
	return &Service{
		store:     statestore.New(fs),
		fs:        fs,
		statePath: filepath.Join(statesDir, "packages.state.json"),
	}
}

func (s *Service) Load() (*PackagesState, error) {
	st := &PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]PkgEntry{}}
	if err := s.store.LoadJSON(s.statePath, st); err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return st, nil
		}
		return nil, err
	}
	if st.Packages == nil {
		st.Packages = map[string]PkgEntry{}
	}
	return st, nil
}

func (s *Service) Save(st *PackagesState) error {
	dir := filepath.Dir(s.statePath)
	if err := s.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}
	s.fs.Chmod(dir, 0755)
	return s.store.SaveJSON(s.statePath, st)
}

func (s *Service) Lookup(st *PackagesState, name string) (PkgEntry, bool) {
	entry, ok := st.Packages[name]
	return entry, ok
}

func (s *Service) IsInstalled(st *PackagesState, name string) bool {
	_, ok := st.Packages[name]
	return ok
}

func (s *Service) Add(st *PackagesState, name string, entry PkgEntry) {
	st.Packages[name] = entry
}

func (s *Service) Remove(st *PackagesState, name string) {
	delete(st.Packages, name)
}
