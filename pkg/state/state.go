package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/writeutil"
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
		if os.IsNotExist(err) || os.IsPermission(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		os.Chmod(s.path, 0644)
	}
	return nil
}

func (s *Store) Save(v any) error {
	if err := os.MkdirAll(settings.Default.StatesDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeutil.AtomicFile(s.path, data, 0644)
}
