package service

import (
	"context"
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
	reg.Register(&pkg.Package{
		Name: "variant-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"real-system-pkg"}},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, statePath, cleanup := newStateManagerForTest(t)

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: runner, fs: fs.NewFileSystem(),
		},
	}

	return svc, statePath, cleanup
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

func TestRemoveOne_variantOnlyPackage(t *testing.T) {
	svc, statePath, cleanup := setupRemoveTest(t, &successRunner{})
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"variant-pkg": {Type: "apt", Variant: "stable"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.RemoveOne(ctx, "variant-pkg", st, spinner); err != nil {
		t.Fatalf("RemoveOne: %v", err)
	}

	if _, ok := st.Packages["variant-pkg"]; ok {
		t.Error("expected variant-pkg removed from in-memory state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["variant-pkg"]; ok {
		t.Error("expected variant-pkg removed from persisted state on disk")
	}
}
