package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- SelectVariants tests ---------------------------------------------------

func TestSelectVariants_noVariantsSkips(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockVariantSelector{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	if err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false); err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}
}

func TestSelectVariants_appliesVariant(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}, "beta": {"pkg-beta"}},
		},
	})

	instReg := installer.NewRegistry()
	sel := &mockVariantSelector{selectedVariant: "beta"}
	instReg.Register(pkg.TypeApt, sel)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	if err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false); err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}

	p, _ := reg.Lookup("test-pkg")
	if p.Apt.Variant != "beta" {
		t.Errorf("expected variant 'beta', got %q", p.Apt.Variant)
	}
}

func TestSelectVariants_skipsWhenInStateAndNotForce(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}},
		},
	})

	instReg := installer.NewRegistry()
	sel := &mockVariantSelector{selectedVariant: "beta"}
	instReg.Register(pkg.TypeApt, sel)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Variant: "stable"},
	}}
	if err := stateSvc.Save(st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	if err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false); err != nil {
		t.Fatalf("SelectVariants: %v", err)
	}

	p, _ := reg.Lookup("test-pkg")
	if p.Apt.Variant != "" {
		t.Errorf("expected variant unchanged (empty), got %q", p.Apt.Variant)
	}
}

func TestSelectVariants_loadError(t *testing.T) {
	stateSvc, statePath, cleanup := newStateManagerForTest(t)
	defer cleanup()
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	svc := &InstallService{
		baseService: baseService{state: stateSvc},
	}

	err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false)
	if err == nil || !strings.Contains(err.Error(), "load state") {
		t.Fatalf("expected 'load state' error, got: %v", err)
	}
}

func TestSelectVariants_lookupError(t *testing.T) {
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:   pkg.NewRegistry(),
			state: stateSvc,
		},
		resolver: NewResolver(pkg.NewRegistry()),
	}

	err := svc.SelectVariants(context.Background(), []string{"nonexistent"}, false)
	if err == nil || !strings.Contains(err.Error(), "unknown package") {
		t.Fatalf("expected 'unknown package' error, got: %v", err)
	}
}

func TestSelectVariants_resolveError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "self-loop",
		Type:    pkg.TypeApt,
		Apt:     &pkg.AptConfig{},
		Depends: []string{"self-loop"},
	})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:   reg,
			state: stateSvc,
		},
		resolver: NewResolver(reg),
	}

	err := svc.SelectVariants(context.Background(), []string{"self-loop"}, false)
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestSelectVariants_lookupInstallerError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
	})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg:     reg,
			instReg: installer.NewRegistry(),
			state:   stateSvc,
		},
		resolver: NewResolver(reg),
	}

	err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false)
	if err == nil || !strings.Contains(err.Error(), "no installer for type") {
		t.Fatalf("expected 'no installer for type' error, got: %v", err)
	}
}

func TestSelectVariants_notAVariantSelector(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
		},
		resolver: NewResolver(reg),
	}

	if err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectVariants_selectError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockVariantSelector{selectedVariant: "", selectErr: os.ErrInvalid})

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
		},
		resolver: NewResolver(reg),
	}

	err := svc.SelectVariants(context.Background(), []string{"test-pkg"}, false)
	if err == nil {
		t.Fatal("expected error from SelectVariant")
	}
}

// ---- processOne tests -------------------------------------------------------

func TestProcessOne_rerunBypassesInstalledCheck(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			fs: testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", true, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when force+rerun reinstall")
	}
	if len(recorder.forceFlags) != 1 || !recorder.forceFlags[0] {
		t.Error("expected one install call with ForceInstall=true")
	}
}

func TestProcessOne_skipVariantDoesNothing(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Variant: "__skip__"},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if updated {
		t.Error("expected updated=false when variant is __skip__")
	}
	if len(recorder.forceFlags) != 0 {
		t.Error("expected no install call")
	}
}

func TestProcessOne_alreadyInstalledReturnsFalse(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{},
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", false, false, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if updated {
		t.Error("expected updated=false when already installed")
	}
	if len(recorder.forceFlags) != 0 {
		t.Error("expected no install call")
	}
}

func TestProcessOne_installedButRemovedReinstalls(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &nopRunner{},
			fs:     testutil.NewMockFileSystem(),
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", true, false, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when force-reinstalling removed package")
	}
	if len(recorder.forceFlags) != 1 || !recorder.forceFlags[0] {
		t.Error("expected one install call with ForceInstall=true")
	}
}

// ---- processAll tests -------------------------------------------------------

func TestProcessAll_emptyNames(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{}, false, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll with empty names: %v", err)
	}
}

func TestProcessAll_unknownName(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"nonexistent"}, false, true, st, spinner, "install", "installed")
	if err == nil {
		t.Fatal("expected error for unknown package name")
	}
	if !strings.Contains(err.Error(), "unknown package") {
		t.Errorf("expected error mentioning 'unknown package', got: %v", err)
	}
}

func TestCheckInstalled_runnerError(t *testing.T) {
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.Canceled
		},
	}

	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}
	st := &State{Packages: map[string]PkgEntry{"test-pkg": {Type: "apt"}}}

	_, err := checkInstalled(ctx, stateSvc, st, "test-pkg", runner, nil, nil, p, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestProcessAll_workDoneMultiple(t *testing.T) {
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
	recorder := &variantRecorder{}
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, false, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}
	if _, ok := st.Packages["pkg-a"]; !ok {
		t.Error("expected pkg-a in state")
	}
	if _, ok := st.Packages["pkg-b"]; !ok {
		t.Error("expected pkg-b in state")
	}
}

func TestProcessAll_nothingToDoMultiple(t *testing.T) {
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

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt", Version: "1.0"},
		"pkg-b": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, false, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}
}

func TestProcessAll_nothingToDoMultipleUpdate(t *testing.T) {
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

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt", Version: "1.0"},
		"pkg-b": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, false, true, st, spinner, "update", "updated")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}
}

func TestInstallServiceRun_success(t *testing.T) {
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
	runner := &nopRunner{}
	mfs := testutil.NewMockFileSystem()

	svc := NewInstallService(reg, instReg, NewResolver(reg), stateSvc, locker, lockPath, runner, mfs, nil)

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.Run(ctx, []string{"test-pkg"}, false, spinner); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if locker.AcquireCount != 1 {
		t.Errorf("expected 1 lock acquire, got %d", locker.AcquireCount)
	}
	if locker.ReleaseCount != 1 {
		t.Errorf("expected 1 lock release, got %d", locker.ReleaseCount)
	}
}

func TestInstallServiceRun_loadError(t *testing.T) {
	stateSvc, statePath, cleanup := newStateManagerForTest(t)
	defer cleanup()
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	svc := NewInstallService(
		pkg.NewRegistry(), installer.NewRegistry(), NewResolver(pkg.NewRegistry()),
		stateSvc, &testutil.MockLocker{}, filepath.Join(t.TempDir(), "lock"),
		&nopRunner{}, testutil.NewMockFileSystem(), nil,
	)

	err := svc.Run(context.Background(), []string{"test-pkg"}, false, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error from load state")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected 'load state' error, got: %v", err)
	}
}
