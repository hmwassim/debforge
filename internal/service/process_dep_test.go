package service

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

// setupSharedDepTest creates a service with three packages:
//
//	root-a (apt) depends-on shared (apt)
//	root-b (apt) depends-on shared (apt)
func setupSharedDepTest(t *testing.T) (*InstallService, *variantRecorder, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "root-a",
		Type:    pkg.TypeApt,
		Depends: []string{"shared"},
	})
	reg.Register(&pkg.Package{
		Name:    "root-b",
		Type:    pkg.TypeApt,
		Depends: []string{"shared"},
	})
	reg.Register(&pkg.Package{
		Name: "shared",
		Type: pkg.TypeApt,
	})

	type callCountRecorder struct {
		variantRecorder
	}
	recorder := &callCountRecorder{}

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc, aptUpdate: nopAptUpdater{}, extrepo: nopExtrepoManager{}, pkgLister: nopPackageLister{}},
		resolver:    NewResolver(reg),
	}

	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	return svc, &recorder.variantRecorder, cleanup
}

func TestProcessOne_forcePropagatesToDeps(t *testing.T) {
	svc, recorder, cleanup := setupDepTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(recorder.forceFlags) != 2 {
		t.Fatalf("expected 2 install calls (root + dep), got %d", len(recorder.forceFlags))
	}

	for i, f := range recorder.forceFlags {
		if !f {
			t.Errorf("install call %d: expected ForceInstall=true, got false", i)
		}
	}
}

func TestProcessOne_forceFalseDoesNotSetForceInstall(t *testing.T) {
	svc, recorder, cleanup := setupDepTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "root", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(recorder.forceFlags) != 2 {
		t.Fatalf("expected 2 install calls (root + dep), got %d", len(recorder.forceFlags))
	}

	for i, f := range recorder.forceFlags {
		if f {
			t.Errorf("install call %d: expected ForceInstall=false, got true", i)
		}
	}
}

func TestProcessOne_forceStateUpdateOnUnchangedDep(t *testing.T) {
	svc, _, cleanup := setupDepTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"root": {Type: "apt", Version: "1.0"},
		"dep":  {Type: "apt", Version: "1.0"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(st.Packages) != 2 {
		t.Fatalf("expected 2 packages in state, got %d", len(st.Packages))
	}
}

func TestProcessAll_sharedDepProcessedOnce(t *testing.T) {
	svc, recorder, cleanup := setupSharedDepTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err := svc.processAll(ctx, []string{"root-a", "root-b"}, false, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processAll: %v", err)
	}

	if len(recorder.forceFlags) != 3 {
		t.Fatalf("expected 3 install calls (root-a + shared + root-b), got %d", len(recorder.forceFlags))
	}

	entry, ok := st.Packages["shared"]
	if !ok {
		t.Fatal("expected shared to be in state")
	}
	if entry.Type != "apt" {
		t.Errorf("expected shared type 'apt', got %q", entry.Type)
	}
}
