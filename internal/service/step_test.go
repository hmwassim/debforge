package service

import (
	"context"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestProcessOne_checkInstalledError(t *testing.T) {
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
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.Canceled
		},
	}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: false, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error from CheckInstalled")
	}
}

func TestProcessOne_resolveError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "test-pkg",
		Type:    pkg.TypeApt,
		Apt:     &pkg.AptConfig{},
		Depends: []string{"test-pkg"},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error from Resolve")
	}
	if !strings.Contains(err.Error(), "dependency cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestProcessOne_lookupInstallerError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error from LookupInstaller")
	}
	if !strings.Contains(err.Error(), "no installer for type") {
		t.Errorf("expected 'no installer for type', got: %v", err)
	}
}

func TestProcessOne_noSaveDuringExecution(t *testing.T) {
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
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	didWork, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !didWork {
		t.Error("expected didWork=true")
	}
	if _, ok := st.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg in in-memory state")
	}
	// processOne no longer persists to disk; that's processAll's job.
}

func TestProcessOne_depAlreadyInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "parent",
		Type:    pkg.TypeApt,
		Apt:     &pkg.AptConfig{},
		Depends: []string{"dep-pkg"},
	})
	reg.Register(&pkg.Package{
		Name: "dep-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &successRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"dep-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	didWork, err := svc.processOne(ctx, "parent", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: false, verb: "install", pastTense: "installed"})
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if !didWork {
		t.Error("expected didWork=true because parent was installed")
	}
}

func TestProcessOne_depCheckInstalledError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "parent",
		Type:    pkg.TypeApt,
		Apt:     &pkg.AptConfig{},
		Depends: []string{"dep-pkg"},
	})
	reg.Register(&pkg.Package{
		Name: "dep-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &variantRecorder{})

	stateSvc, _ := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.Canceled
		},
	}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"dep-pkg": {Type: "apt"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "parent", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: false, verb: "install", pastTense: "installed"})
	if err == nil {
		t.Fatal("expected error from dep CheckInstalled")
	}
}

func TestProcessOne_alreadyUpToDate(t *testing.T) {
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
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = testutil.NewMockFileSystem()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	didWork, err := svc.processOne(ctx, "test-pkg", &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if didWork {
		t.Error("expected didWork=false when version unchanged")
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

	stateSvc, tmpPath:= newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, sys: nil, aptUpdate: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}},
		resolver:    NewResolver(reg),
	}
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"root-a", "root-b"}, &pipelineCtx{st: st, spinner: spinner, force: false, rerun: true, verb: "install", pastTense: "installed"})
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
