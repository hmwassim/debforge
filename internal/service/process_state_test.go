package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
			Variants: map[string][]string{"stable": {"real-system-pkg"}},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, statePath, cleanup := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil},
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

func TestProcessOne_depChainPartialFailurePersistsCompleted(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "root-pkg",
		Type:    pkg.TypeApt,
		Apt:     &pkg.AptConfig{},
		Depends: []string{"dep-a", "dep-b"},
	})
	reg.Register(&pkg.Package{
		Name: "dep-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "dep-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	failInst := &failAfterRecorder{failAfter: 3}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, failInst)

	stateSvc, tmpPath, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "root-pkg", false, true, st, spinner, "install", "installed", nil)
	if err == nil {
		t.Fatal("expected error from processOne due to root-pkg failure")
	}

	if _, ok := st.Packages["dep-a"]; !ok {
		t.Error("expected dep-a in in-memory state after partial failure")
	}
	if _, ok := st.Packages["dep-b"]; !ok {
		t.Error("expected dep-b in in-memory state after partial failure")
	}
	if _, ok := st.Packages["root-pkg"]; ok {
		t.Error("unexpected root-pkg in in-memory state after failure")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), tmpPath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["dep-a"]; !ok {
		t.Error("expected dep-a in persisted state on disk after partial failure")
	}
	if _, ok := loaded.Packages["dep-b"]; !ok {
		t.Error("expected dep-b in persisted state on disk after partial failure")
	}
	if _, ok := loaded.Packages["root-pkg"]; ok {
		t.Error("unexpected root-pkg in persisted state on disk after failure")
	}
}

func TestLoad_corruptState(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, _ = tmpFile.Write([]byte("{invalid json"))
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	st := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(st)

	_, err = stateSvc.Load()
	if err == nil {
		t.Fatal("expected error for corrupt JSON state")
	}
	if errors.Is(err, store.ErrNotFound) {
		t.Fatal("expected a JSON parse error, not ErrNotFound")
	}
}

func TestLoad_nonExistentReturnsEmpty(t *testing.T) {
	st := store.NewStore[State](fs.NewFileSystem(), "/tmp/nonexistent-debforge-test.json")
	stateSvc := NewStateManager(st)

	got, err := stateSvc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Packages == nil {
		t.Fatal("expected non-nil Packages map for empty state")
	}
	if len(got.Packages) != 0 {
		t.Errorf("expected empty Packages, got %d entries", len(got.Packages))
	}
}

func TestSave_fails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "debforge-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write initial state: %v", err)
	}

	if err := os.Chmod(tmpDir, 0500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	st := store.NewStore[State](fs.NewFileSystem(), statePath)
	stateSvc := NewStateManager(st)

	err = stateSvc.Save(&State{Packages: map[string]PkgEntry{"pkg-a": {}}})
	if err == nil {
		t.Fatal("expected error when saving to read-only directory")
	}
}
