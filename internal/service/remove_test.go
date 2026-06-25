package service

import (
	"context"
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func setupRemoveTest(t *testing.T, runner ports.CommandRunner) (*RemoveService, string, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	tmpFile.Close()

	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)

	svc := &RemoveService{
		reg:     reg,
		instReg: instReg,
		state:   stateSvc,
		runner:  runner,
		fs:      fs.NewFileSystem(),
	}

	cleanup := func() { os.Remove(tmpFile.Name()) }
	return svc, tmpFile.Name(), cleanup
}

func TestRemoveOne_successPersistsState(t *testing.T) {
	svc, statePath, cleanup := setupRemoveTest(t, &successRunner{})
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.RemoveOne(ctx, "test-pkg", st, spinner); err != nil {
		t.Fatalf("RemoveOne: %v", err)
	}

	if _, ok := st.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from in-memory state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from persisted state on disk")
	}
}

func TestRemoveOne_staleEntryPersistsCleanup(t *testing.T) {
	svc, statePath, cleanup := setupRemoveTest(t, &nopRunner{})
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.RemoveOne(ctx, "test-pkg", st, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled from RemoveOne for stale entry")
	}

	if _, ok := st.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from in-memory state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from persisted state after stale cleanup")
	}
}
