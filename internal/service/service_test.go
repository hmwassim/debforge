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

// Constructor tests

func TestNewInstallService(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := NewInstallService(reg, instReg, NewResolver(reg), stateSvc, nil, "", nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewRemoveService(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := NewRemoveService(reg, instReg, stateSvc, nil, "", nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// State tests

func TestListPackages(t *testing.T) {
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt"},
		"pkg-b": {Type: "deb"},
	}}
	names := stateSvc.ListPackages(st)
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestListPackages_empty(t *testing.T) {
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	names := stateSvc.ListPackages(st)
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

// pkgIsOrphaned tests

func TestPkgIsOrphaned_nonAptOrDeb(t *testing.T) {
	p := &pkg.Package{Name: "test", Type: pkg.TypeSource}
	if pkgIsOrphaned(p, nil) {
		t.Error("expected false for non-apt/deb type")
	}
}

func TestPkgIsOrphaned_variantSomeInstalled(t *testing.T) {
	p := &pkg.Package{
		Name: "test", Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{Variants: map[string][]string{
			"stable": {"pkg-stable"}, "staging": {"pkg-staging"},
		}},
	}
	if pkgIsOrphaned(p, map[string]bool{"pkg-stable": true}) {
		t.Error("expected false when a variant package is installed")
	}
}

func TestPkgIsOrphaned_variantNoneInstalled(t *testing.T) {
	p := &pkg.Package{
		Name: "test", Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{Variants: map[string][]string{
			"stable": {"pkg-stable"},
		}},
	}
	if !pkgIsOrphaned(p, map[string]bool{}) {
		t.Error("expected true when no variant package is installed")
	}
}

func TestPkgIsOrphaned_noVariantInstalled(t *testing.T) {
	p := &pkg.Package{
		Name: "test", Type: pkg.TypeApt,
		Packages: []string{"real-pkg"},
	}
	if pkgIsOrphaned(p, map[string]bool{"real-pkg": true}) {
		t.Error("expected false when primary package is installed")
	}
}

func TestPkgIsOrphaned_noVariantNotInstalled(t *testing.T) {
	p := &pkg.Package{
		Name: "test", Type: pkg.TypeApt,
		Packages: []string{"real-pkg"},
	}
	if !pkgIsOrphaned(p, map[string]bool{}) {
		t.Error("expected true when primary package is not installed")
	}
}

// extrepoNeeded tests

func TestExtrepoNeeded(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-2"}}})

	svc := &RemoveService{baseService: baseService{reg: reg}}
	st := &State{Packages: map[string]PkgEntry{"pkg-a": {}, "pkg-b": {}}}

	if !svc.extrepoNeeded(context.Background(), "repo-2", "pkg-a", st) {
		t.Error("expected repo-2 needed by pkg-b")
	}
	if svc.extrepoNeeded(context.Background(), "repo-1", "pkg-a", st) {
		t.Error("expected repo-1 not needed after removing pkg-a (it is the only consumer)")
	}
}

func TestExtrepoNeeded_notNeeded(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}})
	// pkg-b has no extrepo
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt})

	svc := &RemoveService{baseService: baseService{reg: reg}}
	st := &State{Packages: map[string]PkgEntry{"pkg-a": {}, "pkg-b": {}}}

	if svc.extrepoNeeded(context.Background(), "repo-1", "pkg-a", st) {
		t.Error("expected repo-1 not needed by any other package")
	}
}

// disableOrphanedExtrepos tests

func TestDisableOrphanedExtrepos(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-1", "repo-2"}}})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}})

	var disabled []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "extrepo" && len(args) >= 2 && args[0] == "disable" {
				disabled = append(disabled, args[1])
			}
			return nil, nil, nil
		},
	}

	svc := &RemoveService{baseService: baseService{reg: reg, runner: runner}}
	p := &pkg.Package{Name: "pkg-a", Apt: &pkg.AptConfig{Extrepo: []string{"repo-1", "repo-2"}}}
	st := &State{Packages: map[string]PkgEntry{"pkg-a": {}, "pkg-b": {}}}

	svc.disableOrphanedExtrepos(context.Background(), p, st, &mockSpinner{})

	if len(disabled) != 1 || disabled[0] != "repo-2" {
		t.Errorf("expected only repo-2 to be disabled, got %v", disabled)
	}
}

func TestDisableOrphanedExtrepos_noApt(t *testing.T) {
	svc := &RemoveService{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeDeb}
	// Should not panic when Apt is nil
	svc.disableOrphanedExtrepos(context.Background(), p, nil, &mockSpinner{})
}

func TestDisableOrphanedExtrepos_error(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}})

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "extrepo" {
				return nil, nil, nil
			}
			return nil, nil, nil
		},
	}

	svc := &RemoveService{baseService: baseService{reg: reg, runner: runner}}
	p := &pkg.Package{Name: "pkg-a", Apt: &pkg.AptConfig{Extrepo: []string{"repo-1"}}}
	st := &State{Packages: map[string]PkgEntry{"pkg-a": {}}}

	// repo-1 not needed by any other (none registered), so it should be disabled
	svc.disableOrphanedExtrepos(context.Background(), p, st, &mockSpinner{})
}

// checkInstalled error path

// ---- SelectVariants tests ---------------------------------------------------

type mockVariantSelector struct {
	variantRecorder
	selectedVariant string
	selectErr       error
}

func (m *mockVariantSelector) SelectVariant(_ context.Context, p *pkg.Package) error {
	if m.selectErr != nil {
		return m.selectErr
	}
	p.Apt.Variant = m.selectedVariant
	return nil
}

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

	// Save state with an existing variant.
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

	_, err := checkInstalled(ctx, stateSvc, st, "test-pkg", runner, nil, p, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestLookupInstaller_unregisteredType(t *testing.T) {
	instReg := installer.NewRegistry()
	_, err := LookupInstaller(instReg, pkg.TypeApt)
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
	if !strings.Contains(err.Error(), "no installer for type") {
		t.Errorf("expected 'no installer for type', got: %v", err)
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

	svc := NewInstallService(reg, instReg, NewResolver(reg), stateSvc, locker, lockPath, runner, mfs)

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

	// variantRecorder does not implement variantSelector, so
	// SelectVariants should silently continue (no error).
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

func TestInstallServiceRun_loadError(t *testing.T) {
	// Create corrupt state file that causes state.Load to fail (not ErrNotFound).
	stateSvc, statePath, cleanup := newStateManagerForTest(t)
	defer cleanup()
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	svc := NewInstallService(
		pkg.NewRegistry(), installer.NewRegistry(), NewResolver(pkg.NewRegistry()),
		stateSvc, &testutil.MockLocker{}, filepath.Join(t.TempDir(), "lock"),
		&nopRunner{}, testutil.NewMockFileSystem(),
	)

	err := svc.Run(context.Background(), []string{"test-pkg"}, false, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error from load state")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected 'load state' error, got: %v", err)
	}
}
