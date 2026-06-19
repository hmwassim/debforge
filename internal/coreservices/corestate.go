package services

import (
	"context"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/statestore"
	"github.com/hmwassim/debforge/internal/ports"
)

type coreState struct {
	statestore.Versioned
	LastSetupCommit string   `json:"last_setup_commit,omitempty"`
	ManagedPackages []string `json:"managed_packages,omitempty"`
	ManagedConfigs  []string `json:"managed_configs,omitempty"`
}

type CoreStateStore struct {
	store  *statestore.Store
	runner ports.CommandRunner
	logger ports.UI
	path   string
}

func NewCoreStateStore(fs ports.FileSystem, runner ports.CommandRunner, logger ports.UI, path string) *CoreStateStore {
	return &CoreStateStore{
		store:  statestore.New(fs),
		runner: runner,
		logger: logger,
		path:   path,
	}
}

func (s *CoreStateStore) Load() *coreState {
	st := &coreState{Versioned: statestore.DefaultVersioned}
	if err := s.store.LoadJSON(s.path, st); err != nil {
		if !os.IsNotExist(err) {
			s.logger.Warn("Core state file corrupt, resetting: %v", err)
		}
		return &coreState{Versioned: statestore.DefaultVersioned}
	}
	if st.IsLegacy() {
		s.logger.Warn("Core state file has no version (legacy format), migrating")
		st.Version = statestore.CurrentVersion
	}
	return st
}

func (s *CoreStateStore) Save(st *coreState) error {
	st.Version = statestore.CurrentVersion
	return s.store.SaveJSON(s.path, st)
}

func (s *CoreStateStore) CurrentCommit(ctx context.Context, sourceDir string) string {
	stdout, _, err := s.runner.Run(ctx, "git", "-C", sourceDir, "rev-parse", "HEAD")
	if err != nil {
		s.logger.Warn("Could not check source commit: %v", err)
		return ""
	}
	return strings.TrimSpace(string(stdout))
}

func setDiff(prev, cur []string) []string {
	if len(prev) == 0 {
		return nil
	}
	curSet := make(map[string]bool, len(cur))
	for _, s := range cur {
		curSet[s] = true
	}
	var diff []string
	for _, s := range prev {
		if !curSet[s] {
			diff = append(diff, s)
		}
	}
	return diff
}
