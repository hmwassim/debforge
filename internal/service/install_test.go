package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/adapters/lock"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestSelectVariants(t *testing.T) {
	tests := []struct {
		name            string
		pkgDefs         []pkg.Package
		instSelector    installer.Installer
		statePkgs       map[string]PkgEntry
		corruptState    bool
		wantErr         bool
		wantErrContains []string
		wantVariant     string
	}{
		{
			name:    "no variants skips",
			pkgDefs: []pkg.Package{{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}},
			instSelector: &mockVariantSelector{},
		},
		{
			name: "applies variant",
			pkgDefs: []pkg.Package{{
				Name: "test-pkg",
				Type: pkg.TypeApt,
				Apt:  &pkg.AptConfig{Variants: map[string][]string{"stable": {"pkg-stable"}, "beta": {"pkg-beta"}}},
			}},
			instSelector: &mockVariantSelector{selectedVariant: "beta"},
			wantVariant:  "beta",
		},
		{
			name: "skips when in state and not force",
			pkgDefs: []pkg.Package{{
				Name: "test-pkg",
				Type: pkg.TypeApt,
				Apt:  &pkg.AptConfig{Variants: map[string][]string{"stable": {"pkg-stable"}}},
			}},
			instSelector: &mockVariantSelector{selectedVariant: "beta"},
			statePkgs:    map[string]PkgEntry{"test-pkg": {Type: "apt", Variant: "stable"}},
			wantVariant:  "",
		},
		{
			name:         "load error",
			corruptState: true,
			wantErr:      true,
			wantErrContains: []string{"load state"},
		},
		{
			name:            "lookup error",
			pkgDefs:         nil,
			wantErr:         true,
			wantErrContains: []string{"unknown package"},
		},
		{
			name: "resolve error",
			pkgDefs: []pkg.Package{{
				Name:    "self-loop",
				Type:    pkg.TypeApt,
				Apt:     &pkg.AptConfig{},
				Depends: []string{"self-loop"},
			}},
			wantErr:         true,
			wantErrContains: []string{"dependency cycle"},
		},
		{
			name: "lookup installer error",
			pkgDefs: []pkg.Package{{
				Name: "test-pkg",
				Type: pkg.TypeApt,
				Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
			}},
			wantErr:         true,
			wantErrContains: []string{"no installer for type"},
		},
		{
			name: "not a variant selector",
			pkgDefs: []pkg.Package{{
				Name: "test-pkg",
				Type: pkg.TypeApt,
				Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
			}},
			instSelector: &variantRecorder{},
		},
		{
			name: "select error",
			pkgDefs: []pkg.Package{{
				Name: "test-pkg",
				Type: pkg.TypeApt,
				Apt:  &pkg.AptConfig{Variants: map[string][]string{"a": {"pkg-a"}}},
			}},
			instSelector: &mockVariantSelector{selectedVariant: "", selectErr: os.ErrInvalid},
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := pkg.NewRegistry()
			for i := range tc.pkgDefs {
				p := tc.pkgDefs[i]
				reg.Register(&p)
			}

			instReg := installer.NewRegistry()
			if tc.instSelector != nil {
				instReg.Register(pkg.TypeApt, tc.instSelector)
			}

			stateSvc, statePath := newStateManagerForTest(t)

			if tc.corruptState {
				if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
					t.Fatalf("write corrupt state: %v", err)
				}
			}

			if tc.statePkgs != nil {
				st := &State{Packages: tc.statePkgs}
				if err := stateSvc.Save(st); err != nil {
					t.Fatalf("save state: %v", err)
				}
			}

			svc := &InstallService{
				baseService: baseService{
					reg:       reg,
					instReg:   instReg,
					state:     stateSvc,
					aptUpdate: testutil.NopAptUpdater{},
					extrepo:   testutil.NopExtrepoManager{},
				},
				resolver: NewResolver(reg),
			}

			var names []string
			for _, p := range tc.pkgDefs {
				names = append(names, p.Name)
			}
			if len(names) == 0 {
				names = []string{"nonexistent"}
			}

			err := svc.SelectVariants(context.Background(), names, false)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, substr := range tc.wantErrContains {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("expected error containing %q, got: %v", substr, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantVariant != "" {
				p, _ := reg.Lookup(names[0])
				if p.Apt.Variant != tc.wantVariant {
					t.Errorf("expected variant %q, got %q", tc.wantVariant, p.Apt.Variant)
				}
			}
		})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			fs: testutil.NewMockFileSystem(),
			aptUpdate:  testutil.NopAptUpdater{},
			extrepo:    testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: true, rerun: true, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{},
			fs:     testutil.NewMockFileSystem(),
			aptUpdate:  testutil.NopAptUpdater{},
			extrepo:    testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: false, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &nopRunner{},
			fs:     testutil.NewMockFileSystem(),
			aptUpdate:  testutil.NopAptUpdater{},
			extrepo:    testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	updated, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: true, rerun: false, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err != nil {
		t.Fatalf("processAll with empty names: %v", err)
	}
}

func TestProcessAll_unknownName(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"nonexistent"}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error for unknown package name")
	}
	if !strings.Contains(err.Error(), "unknown package") {
		t.Errorf("expected error mentioning 'unknown package', got: %v", err)
	}
}

func TestCheckInstalled_runnerError(t *testing.T) {
	stateSvc, _ := newStateManagerForTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.Canceled
		},
	}

	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}
	st := &State{Packages: map[string]PkgEntry{"test-pkg": {Type: "apt"}}}

	svc := &baseService{state: stateSvc, runner: runner, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}}
	_, err := svc.checkInstalled(ctx, st, "test-pkg", p, &mockSpinner{})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
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

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
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

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
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

	err := svc.processAll(ctx, []string{"pkg-a", "pkg-b"}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "update", pastTense: "updated"})
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

	stateSvc, _ := newStateManagerForTest(t)

	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")
	runner := &nopRunner{}
	mfs := testutil.NewMockFileSystem()

	svc := NewInstallService(Deps{Reg: reg, InstReg: instReg, State: stateSvc, Locker: locker, LockPath: lockPath, Runner: runner, Fs: mfs}, NewResolver(reg))

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
	stateSvc, statePath := newStateManagerForTest(t)
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	svc := NewInstallService(Deps{
		Reg: pkg.NewRegistry(), InstReg: installer.NewRegistry(), State: stateSvc,
		Locker: &testutil.MockLocker{}, LockPath: filepath.Join(t.TempDir(), "lock"),
		Runner: &nopRunner{}, Fs: testutil.NewMockFileSystem(),
	}, NewResolver(pkg.NewRegistry()))

	err := svc.Run(context.Background(), []string{"test-pkg"}, false, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error from load state")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected 'load state' error, got: %v", err)
	}
}

// ---- shouldSkip tests -------------------------------------------------------

func TestShouldSkip_skipUpdateAlreadyInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})
	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &successRunner{},
			fs:     testutil.NewMockFileSystem(),
			sys:    &mockSystem{homeDir: "/home/test"},
			aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	dep := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}, SkipUpdate: true}
	st := &State{Packages: map[string]PkgEntry{"test-pkg": {Type: "apt"}}}
	processed := map[string]bool{}

	skip, err := svc.shouldSkip(context.Background(), dep, true, &pipelineCtx{st: st, spinner: &mockSpinner{}, rerun: false, verb: "install", sessionProcessed: processed})
	if err != nil {
		t.Fatalf("shouldSkip: %v", err)
	}
	if !skip {
		t.Error("expected skip=true when SkipUpdate=true and package is installed")
	}
	if !processed["test-pkg"] {
		t.Error("expected test-pkg to be marked as processed")
	}
}

func TestShouldSkip_skipUpdateNotInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})
	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &nopRunner{},
			fs:     testutil.NewMockFileSystem(),
			sys:    &mockSystem{homeDir: "/home/test"},
			aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	dep := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}, SkipUpdate: true}
	st := &State{Packages: map[string]PkgEntry{}}
	processed := map[string]bool{}

	skip, err := svc.shouldSkip(context.Background(), dep, false, &pipelineCtx{st: st, spinner: &mockSpinner{}, rerun: false, verb: "install", sessionProcessed: processed})
	if err != nil {
		t.Fatalf("shouldSkip: %v", err)
	}
	if skip {
		t.Error("expected skip=false when SkipUpdate=true but package is not installed")
	}
}

func TestShouldSkip_variantSkip(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Variant: "__skip__"},
	})
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})
	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	dep := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{Variant: "__skip__"}}
	st := &State{Packages: map[string]PkgEntry{}}
	processed := map[string]bool{}

	skip, err := svc.shouldSkip(context.Background(), dep, false, &pipelineCtx{st: st, spinner: &mockSpinner{}, rerun: false, verb: "install", sessionProcessed: processed})
	if err != nil {
		t.Fatalf("shouldSkip: %v", err)
	}
	if !skip {
		t.Error("expected skip=true when variant is __skip__")
	}
}

// ---- context cancellation tests ---------------------------------------------

func TestProcessAll_cancelledContext(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})
	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: stateSvc,
			runner: &testutil.MockRunner{
				RunFunc: func(ctx context.Context, _ string, _ ...string) ([]byte, []byte, error) {
					return nil, nil, ctx.Err()
				},
			},
			fs:     testutil.NewMockFileSystem(),
			aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{},
		},
		resolver: NewResolver(reg),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	st := &State{Packages: map[string]PkgEntry{"test-pkg": {Type: "apt", Version: "1.0"}}}
	err := svc.processAll(ctx, []string{"test-pkg"}, &pipelineCtx{st: st, spinner: &mockSpinner{}, force: false, rerun: false, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---- concurrency tests -------------------------------------------------------

// concurrencyInstaller tracks the max number of concurrent Install calls
// so we can verify the file lock serializes concurrent Run invocations.
type concurrencyInstaller struct {
	maxConcurrent atomic.Int32
	active        atomic.Int32
}

func (c *concurrencyInstaller) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	cur := c.active.Add(1)
	for {
		old := c.maxConcurrent.Load()
		if cur <= old || c.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(50 * time.Millisecond)
	c.active.Add(-1)
	return nil
}

func (c *concurrencyInstaller) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

func TestInstallServiceRun_concurrentSerialization(t *testing.T) {
	t.Parallel()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}})

	tracker := &concurrencyInstaller{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, tracker)

	stateSvc, _ := newStateManagerForTest(t)
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewInstallService(Deps{
		Reg: reg, InstReg: instReg, State: stateSvc,
		Locker: &lock.FLock{}, LockPath: lockPath,
		Runner: &nopRunner{}, Fs: testutil.NewMockFileSystem(),
		AptUpd: testutil.NopAptUpdater{}, Extrepo: testutil.NopExtrepoManager{},
	}, NewResolver(reg))

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)

	for _, name := range []string{"pkg-a", "pkg-b"} {
		go func(n string) {
			defer wg.Done()
			errs <- svc.Run(context.Background(), []string{n}, false, &mockSpinner{})
		}(name)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	}

	if got := tracker.maxConcurrent.Load(); got > 1 {
		t.Errorf("max concurrent Install calls = %d, want ≤ 1 — file lock is not serializing", got)
	}
}

// ---- Update with named packages tests ----------------------------------------

func TestUpdate_namedPackages(t *testing.T) {
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

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _ := newStateManagerForTest(t)
	locker := &testutil.MockLocker{}
	lockPath := filepath.Join(t.TempDir(), "lock")

	svc := NewInstallService(Deps{
		Reg: reg, InstReg: instReg, State: stateSvc, Locker: locker, LockPath: lockPath,
		Runner: &successRunner{}, Fs: testutil.NewMockFileSystem(), Sys: &mockSystem{homeDir: "/home/test"},
	}, NewResolver(reg))

	st := &State{Packages: map[string]PkgEntry{
		"pkg-a": {Type: "apt", Version: "1.0"},
	}}
	if err := stateSvc.Save(st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	ctx := context.Background()
	spinner := &mockSpinner{}

	if err := svc.Update(ctx, []string{"pkg-a"}, false, false, spinner); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(recorder.forceFlags) != 1 {
		t.Errorf("expected 1 install call for pkg-a only, got %d", len(recorder.forceFlags))
	}
}
