package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hmwassim/debforge/pkg/settings"
)

const CurrentVersion = 1

type State struct {
	Version     int    `json:"_version"`
	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func New() *State {
	return &State{Version: CurrentVersion}
}

func Load() (*State, error) {
	s := New()
	data, err := os.ReadFile(settings.Default.StateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *State) migrate() error {
	for s.Version < CurrentVersion {
		next := s.Version + 1
		switch next {
		case 1:
			// v0 → v1: initial schema, no structural changes
			s.Version = 1
		default:
			return fmt.Errorf("state version %d: unknown migration path to version %d", s.Version, next)
		}
	}
	return nil
}

func (s *State) Save() error {
	s.Version = CurrentVersion
	if err := settings.Default.EnsureDirsExist(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settings.Default.StateFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, settings.Default.StateFile())
}
