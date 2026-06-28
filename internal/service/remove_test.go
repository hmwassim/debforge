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

func TestRemoveOne_removesTransitiveDependents(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "scx-scheds",
		Type: pkg.TypeDeb,
		Deb:  &pkg.DebConfig{Package: "scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-tools",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-tools"},
		Depends: []string{"scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-switcher",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-switcher"},
		Depends: []string{"scx-scheds", "scx-tools"},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeDeb, &variantRecorder{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &dpkgRunner{installed: []string{"scx-scheds", "scx-tools", "scx-switcher"}},
			fs:     fs.NewFileSystem(),
		},
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	t.Run("leaf removal does not affect dependents", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds":   {Type: "deb"},
			"scx-tools":    {Type: "deb"},
			"scx-switcher": {Type: "deb"},
		}}
		if err := svc.RemoveOne(ctx, "scx-switcher", st, spinner); err != nil {
			t.Fatalf("RemoveOne(scx-switcher): %v", err)
		}
		if _, ok := st.Packages["scx-scheds"]; !ok {
			t.Error("scx-scheds should remain installed")
		}
		if _, ok := st.Packages["scx-tools"]; !ok {
			t.Error("scx-tools should remain installed")
		}
		if _, ok := st.Packages["scx-switcher"]; ok {
			t.Error("scx-switcher should be removed")
		}
	})

	t.Run("middle removal removes direct dependents", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds":   {Type: "deb"},
			"scx-tools":    {Type: "deb"},
			"scx-switcher": {Type: "deb"},
		}}
		if err := svc.RemoveOne(ctx, "scx-tools", st, spinner); err != nil {
			t.Fatalf("RemoveOne(scx-tools): %v", err)
		}
		if _, ok := st.Packages["scx-scheds"]; !ok {
			t.Error("scx-scheds should remain installed")
		}
		if _, ok := st.Packages["scx-tools"]; ok {
			t.Error("scx-tools should be removed")
		}
		if _, ok := st.Packages["scx-switcher"]; ok {
			t.Error("scx-switcher should be removed (depends on scx-tools)")
		}
	})

	t.Run("root removal removes all transitive dependents", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds":   {Type: "deb"},
			"scx-tools":    {Type: "deb"},
			"scx-switcher": {Type: "deb"},
		}}
		if err := svc.RemoveOne(ctx, "scx-scheds", st, spinner); err != nil {
			t.Fatalf("RemoveOne(scx-scheds): %v", err)
		}
		if _, ok := st.Packages["scx-scheds"]; ok {
			t.Error("scx-scheds should be removed")
		}
		if _, ok := st.Packages["scx-tools"]; ok {
			t.Error("scx-tools should be removed (depends on scx-scheds)")
		}
		if _, ok := st.Packages["scx-switcher"]; ok {
			t.Error("scx-switcher should be removed (depends on scx-scheds)")
		}
	})
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
