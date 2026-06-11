package state

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/hmwassim/debforge/pkg/settings"
)

type State struct {
	mu sync.Mutex

	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func New() *State {
	return &State{}
}

func Load() (*State, error) {
	s := New()
	data, err := os.ReadFile(settings.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := settings.EnsureDirsExist(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settings.StateFile, data, 0644)
}
