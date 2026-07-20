package service

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
)

func TestCheckInstalled_doesNotPersist(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}

	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}
	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := svc.checkInstalled(ctx, st, "test-pkg", p, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled from checkInstalled")
	}
	if !cleanedUp {
		t.Error("expected cleanedUp=true for stale entry")
	}

	if _, ok := st.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from in-memory state by checkInstalled")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in persisted state (checkInstalled must not persist)")
	}
}

func TestCheckInstalled_installed(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}

	svc.runner = &successRunner{}

	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := svc.checkInstalled(ctx, st, "test-pkg", p, spinner)
	if err != nil {
		t.Fatalf("expected no error for installed package, got: %v", err)
	}
	if cleanedUp {
		t.Error("expected cleanedUp=false for installed package")
	}
	if _, ok := st.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in in-memory state")
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

func TestCheckInstalled_notInstalled(t *testing.T) {
	svc, statePath:= setupPersistenceTest(t)

	st := &State{Packages: map[string]PkgEntry{}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}
	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := svc.checkInstalled(ctx, st, "test-pkg", p, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled for missing package")
	}
	if cleanedUp {
		t.Error("expected cleanedUp=false for package not in state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if len(loaded.Packages) != 0 {
		t.Error("expected persisted state to remain empty")
	}
}
