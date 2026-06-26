package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type failAfterRecorder struct {
	variantRecorder
	failAfter int
	callCount int
}

func (r *failAfterRecorder) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	r.callCount++
	if r.callCount >= r.failAfter {
		return fmt.Errorf("simulated failure on %s", p.Name)
	}
	return r.variantRecorder.Install(ctx, p, spinner)
}

func setupPersistenceTest(t *testing.T) (*InstallService, string, func()) {
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
			Variants: map[string]string{"stable": "real-system-pkg"},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, statePath, cleanup := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	return svc, statePath, cleanup
}

func TestProcessOne_successPersistsState(t *testing.T) {
	svc, statePath, cleanup := setupPersistenceTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	entry, ok := st.Packages["test-pkg"]
	if !ok {
		t.Fatal("expected test-pkg in in-memory state")
	}
	if entry.Type != "apt" {
		t.Errorf("expected type 'apt', got %q", entry.Type)
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	diskEntry, ok := loaded.Packages["test-pkg"]
	if !ok {
		t.Fatal("expected test-pkg in persisted state on disk")
	}
	if diskEntry.Type != "apt" {
		t.Errorf("expected persisted type 'apt', got %q", diskEntry.Type)
	}
}

func TestProcessAll_partialFailurePersistsCompleted(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "root-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "root-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	failInst := &failAfterRecorder{failAfter: 2}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, failInst)

	stateSvc, tmpPath, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"root-a", "root-b"}, false, true, st, spinner, "install", "installed")
	if err == nil {
		t.Fatal("expected error from processAll due to root-b failure")
	}

	if _, ok := st.Packages["root-a"]; !ok {
		t.Error("expected root-a in in-memory state after partial failure")
	}
	if _, ok := st.Packages["root-b"]; ok {
		t.Error("unexpected root-b in in-memory state after failure")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), tmpPath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["root-a"]; !ok {
		t.Error("expected root-a in persisted state on disk after partial failure")
	}
	if _, ok := loaded.Packages["root-b"]; ok {
		t.Error("unexpected root-b in persisted state on disk after failure")
	}
}
