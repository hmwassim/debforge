package state

import (
	"encoding/json"
	"os"

	"github.com/hmwassim/debforge/pkg/settings"
)

type State struct {
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
	if err := settings.EnsureDirsExist(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settings.StateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, settings.StateFile)
}
