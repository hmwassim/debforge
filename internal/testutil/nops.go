package testutil

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

// NopAptUpdater is a no-op apt updater for tests.
type NopAptUpdater struct{}

func (NopAptUpdater) RunUpdate(_ context.Context, _ ports.Spinner) error { return nil }

// NopExtrepoManager is a no-op extrepo manager for tests.
type NopExtrepoManager struct{}

func (NopExtrepoManager) NeedsEnable(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (NopExtrepoManager) Enable(_ context.Context, _ string, _ ports.Spinner) error { return nil }

// NopPackageLister is a no-op package lister for tests.
type NopPackageLister struct{}

func (NopPackageLister) ListInstalled(_ context.Context) (map[string]bool, error) {
	return make(map[string]bool), nil
}
