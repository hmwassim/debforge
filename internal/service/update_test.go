package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestUpdate_allCleansUpStaleEntryInMemory(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	svc.locker = &testutil.MockLocker{}
	svc.lockPath = filepath.Join(t.TempDir(), "lock")

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.Update(ctx, nil, false, true, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled from Update for stale entry")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("expected ErrNotInstalled, got: %v", err)
	}

	// Stale cleanup is not persisted to disk.
	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg to remain on disk (cleanup is transient)")
	}
}

func TestUpdate_allWithNoStaleEntries(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	svc.locker = &testutil.MockLocker{}
	svc.lockPath = filepath.Join(t.TempDir(), "lock")

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	svc.runner = &successRunner{}

	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.Update(ctx, nil, false, true, spinner)
	if err != nil {
		t.Fatalf("expected no error for installed package, got: %v", err)
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in persisted state")
	}
}

func TestUpdate_allVariantOnlyPackage(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	svc.locker = &testutil.MockLocker{}
	svc.lockPath = filepath.Join(t.TempDir(), "lock")

	st := &State{Packages: map[string]PkgEntry{
		"variant-pkg": {Type: "apt", Variant: "stable"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	svc.runner = &successRunner{}

	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.Update(ctx, nil, false, true, spinner)
	if err != nil {
		t.Fatalf("expected no error for variant-only package, got: %v", err)
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["variant-pkg"]; !ok {
		t.Error("expected variant-pkg still in persisted state")
	}
}
