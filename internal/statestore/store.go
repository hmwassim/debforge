package statestore

import (
	"encoding/json"
	"os"

	"github.com/hmwassim/debforge/internal/ports"
)

const CurrentVersion = 1

type Versioned struct {
	Version int `json:"version"`
}

func (v *Versioned) IsLegacy() bool {
	return v.Version == 0
}

var DefaultVersioned = Versioned{Version: CurrentVersion}

type Store struct {
	fs ports.FileSystem
}

func New(fs ports.FileSystem) *Store {
	return &Store{fs: fs}
}

func (s *Store) LoadJSON(path string, v any) error {
	data, err := s.fs.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *Store) SaveJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.fs.AtomicWriteFile(path, data, 0644)
}

func (s *Store) Load(path string, v any) (bool, error) {
	err := s.LoadJSON(path, v)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
