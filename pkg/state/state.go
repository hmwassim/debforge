package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/pkg/settings"
)

type Store struct {
	path string
}

func New(namespace string) *Store {
	return &Store{path: filepath.Join(settings.Default.StatesDir(), namespace+".state.json")}
}

func (s *Store) Load(v any) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *Store) Save(v any) error {
	if err := os.MkdirAll(settings.Default.StatesDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
