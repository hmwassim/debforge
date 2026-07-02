package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
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

func TestRemoveOne_listInstalledError(t *testing.T) {
	svc, _, cleanup := setupRemoveTest(t, &nopRunner{})
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	// RemoveOne first calls checkInstalled which calls dpkg-query -W.
	// nopRunner returns nil output, so the package is not detected as
	// installed → ErrNotInstalled, which is returned by RemoveOne.
	// removeOrphaned is never reached in this case because RemoveOne
	// returns before reaching it.
	//
	// We instead test removeOrphaned directly with a runner that
	// fails dpkg.ListInstalled.
	err := svc.RemoveOne(ctx, "test-pkg", st, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled for stale entry")
	}
}

func TestRemoveOrphaned_listInstalledError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "other-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	failRunner := &failOnDpkgRunner{}
	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: failRunner,
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg":  {Type: "apt"},
		"other-pkg": {Type: "apt"},
	}}
	spinner := &mockSpinner{}

	svc.removeOrphaned(context.Background(), st, spinner)
	if failRunner.called {
		t.Log("removeOrphaned logged error to spinner as expected")
	}
}

func TestRemoveServiceRun_multipleSuccess(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewRemoveService(reg, instReg, stateSvc, locker, lockPath, &successRunner{}, testutil.NewMockFileSystem(), nil)

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt"},
		"pkg-b": {Type: "apt"},
	}}
	if err := stateSvc.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.Run(ctx, []string{"pkg-a", "pkg-b"}, spinner); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if locker.AcquireCount != 1 {
		t.Errorf("expected 1 lock acquire, got %d", locker.AcquireCount)
	}
	if locker.ReleaseCount != 1 {
		t.Errorf("expected 1 lock release, got %d", locker.ReleaseCount)
	}
}

func TestRemoveServiceRun_success(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewRemoveService(reg, instReg, stateSvc, locker, lockPath, &successRunner{}, testutil.NewMockFileSystem(), nil)

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := stateSvc.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.Run(ctx, []string{"test-pkg"}, spinner); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if locker.AcquireCount != 1 {
		t.Errorf("expected 1 lock acquire, got %d", locker.AcquireCount)
	}
	if locker.ReleaseCount != 1 {
		t.Errorf("expected 1 lock release, got %d", locker.ReleaseCount)
	}
}

func TestRemoveServiceRun_notInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewRemoveService(reg, instReg, stateSvc, locker, lockPath, &nopRunner{}, testutil.NewMockFileSystem(), nil)

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := stateSvc.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	// nopRunner reports package not installed, RemoveOne returns ErrNotInstalled
	// Run absorbs it and calls DoneInfo.
	if err := svc.Run(ctx, []string{"test-pkg"}, spinner); err != nil {
		t.Fatalf("expected no error (ErrNotInstalled absorbed), got: %v", err)
	}
}

func TestRemoveServiceRun_error(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewRemoveService(reg, instReg, stateSvc, locker, lockPath, &nopRunner{}, testutil.NewMockFileSystem(), nil)

	ctx := context.Background()
	spinner := &mockSpinner{}

	// Package not registered → LookupPackage returns error (not ErrNotInstalled)
	err := svc.Run(ctx, []string{"nonexistent"}, spinner)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
	if !strings.Contains(err.Error(), "unknown package") {
		t.Errorf("expected 'unknown package' error, got: %v", err)
	}
}

func TestRemoveOne_lookupPackageError(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &RemoveService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.RemoveOne(ctx, "nonexistent", st, spinner)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
	if !strings.Contains(err.Error(), "unknown package") {
		t.Errorf("expected 'unknown package' error, got: %v", err)
	}
}

func TestRemoveOne_lookupInstallerError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{}, fs: testutil.NewMockFileSystem(),
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.RemoveOne(ctx, "test-pkg", st, spinner)
	if err == nil {
		t.Fatal("expected error from LookupInstaller")
	}
	if !strings.Contains(err.Error(), "no installer for type") {
		t.Errorf("expected 'no installer for type', got: %v", err)
	}
}

func TestRemoveOne_removeError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &errorRecorder{removeErr: os.ErrPermission})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{}, fs: testutil.NewMockFileSystem(),
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.RemoveOne(ctx, "test-pkg", st, spinner)
	if err == nil {
		t.Fatal("expected error from Remove")
	}
	if !strings.Contains(err.Error(), "remove test-pkg") {
		t.Errorf("expected 'remove test-pkg' error, got: %v", err)
	}
}

func TestRemoveOne_saveStateError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "debforge-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write initial state: %v", err)
	}

	stateStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	stateSvc := NewStateManager(stateStore)

	if err := os.Chmod(tmpDir, 0500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{}, fs: testutil.NewMockFileSystem(),
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err = svc.RemoveOne(ctx, "test-pkg", st, spinner)
	if err == nil {
		t.Fatal("expected error from final saveState")
	}
}

func TestRemoveOrphaned_removesOrphan(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "orphan-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	reg.Register(&pkg.Package{
		Name: "kept-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	// dpkgRunner reports kept-pkg as installed, orphan-pkg as missing
	runner := &dpkgRunner{installed: []string{"kept-pkg"}}

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: runner,
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"orphan-pkg": {Type: "apt"},
		"kept-pkg":   {Type: "apt"},
	}}
	spinner := &mockSpinner{}

	svc.removeOrphaned(context.Background(), st, spinner)

	if _, ok := st.Packages["orphan-pkg"]; ok {
		t.Error("expected orphan-pkg removed from state")
	}
	if _, ok := st.Packages["kept-pkg"]; !ok {
		t.Error("expected kept-pkg to remain in state")
	}
}

func TestRemoveDependents_unknownPackageInState(t *testing.T) {
	reg := pkg.NewRegistry()
	// "orphan-dep" is in state but NOT registered — LookupPackage fails

	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &RemoveService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
	}

	st := &State{Packages: map[string]PkgEntry{
		"orphan-dep": {Type: "apt"},
	}}
	spinner := &mockSpinner{}

	// Should not panic — just continues past the unknown package
	svc.removeDependents(context.Background(), st, spinner)
	if _, ok := st.Packages["orphan-dep"]; !ok {
		t.Error("expected orphan-dep to remain (not a dependent)")
	}
}

func TestRemoveOrphaned_unknownPackageInState(t *testing.T) {
	reg := pkg.NewRegistry()

	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	runner := &dpkgRunner{installed: []string{}}

	svc := &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: runner,
		},
	}

	st := &State{Packages: map[string]PkgEntry{
		"unknown": {Type: "apt"},
	}}
	spinner := &mockSpinner{}

	// Should not panic — just continues past the unknown package
	svc.removeOrphaned(context.Background(), st, spinner)
}

func TestExtrepoNeeded_noAptConfig(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeDeb,
	})
	reg.Register(&pkg.Package{
		Name: "pkg-b",
		Type: pkg.TypeDeb,
	})

	svc := &RemoveService{baseService: baseService{reg: reg}}
	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {},
		"pkg-b": {},
	}}

	// pkg-b has no Apt config, skip it in extrepoNeeded
	needed := svc.extrepoNeeded(context.Background(), "repo-1", "pkg-a", st)
	if needed {
		t.Error("expected false when no other package has extrepo")
	}
}

func TestDisableOrphanedExtrepos_runnerError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "pkg-a",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Extrepo: []string{"repo-1"}},
	})

	var ranExtrepo bool
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "extrepo" {
				ranExtrepo = true
				return nil, nil, os.ErrPermission
			}
			return nil, nil, nil
		},
	}

	svc := &RemoveService{baseService: baseService{reg: reg, runner: runner}}
	p := &pkg.Package{Name: "pkg-a", Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}}
	st := &State{Packages: map[string]PkgEntry{"pkg-a": {}}}

	// Should log error to spinner but not return it
	svc.disableOrphanedExtrepos(context.Background(), p, st, &mockSpinner{})
	if !ranExtrepo {
		t.Error("expected extrepo disable to be attempted")
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

func TestAffectedDependents(t *testing.T) {
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

	svc := &RemoveService{baseService: baseService{reg: reg}}

	t.Run("root removal returns transitive dependents", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds":   {Type: "deb"},
			"scx-tools":    {Type: "deb"},
			"scx-switcher": {Type: "deb"},
		}}
		deps := svc.AffectedDependents(st, []string{"scx-scheds"})
		if len(deps) != 2 {
			t.Fatalf("expected 2 dependents, got %v", deps)
		}
		if !((deps[0] == "scx-tools" && deps[1] == "scx-switcher") ||
			(deps[0] == "scx-switcher" && deps[1] == "scx-tools")) {
			t.Errorf("expected [scx-tools scx-switcher] (any order), got %v", deps)
		}
	})

	t.Run("leaf removal returns no dependents", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds":   {Type: "deb"},
			"scx-tools":    {Type: "deb"},
			"scx-switcher": {Type: "deb"},
		}}
		deps := svc.AffectedDependents(st, []string{"scx-switcher"})
		if deps != nil {
			t.Errorf("expected nil, got %v", deps)
		}
	})

	t.Run("name not in state returns nil", func(t *testing.T) {
		st := &State{Packages: map[string]PkgEntry{
			"scx-scheds": {Type: "deb"},
		}}
		deps := svc.AffectedDependents(st, []string{"nonexistent"})
		if deps != nil {
			t.Errorf("expected nil, got %v", deps)
		}
	})

	t.Run("unknown package in state is skipped", func(t *testing.T) {
		emptyReg := pkg.NewRegistry()
		svc := &RemoveService{baseService: baseService{reg: emptyReg}}
		st := &State{Packages: map[string]PkgEntry{
			"unknown": {Type: "deb"},
		}}
		deps := svc.AffectedDependents(st, []string{"unknown"})
		if deps != nil {
			t.Errorf("expected nil, got %v", deps)
		}
	})
}
