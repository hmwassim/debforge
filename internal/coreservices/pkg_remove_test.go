package services

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/statestore"
)

func newTestRemoveSvc(pkgReg *pkg.Registry, instReg *installers.Registry, stateSvc *state.Service) *RemoveService {
	return NewRemoveService(pkgReg, instReg, stateSvc, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
}

func TestRemoveUnknownPackage(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	stateSvc := state.NewService(newMemFS(), "/tmp/states")
	svc := newTestRemoveSvc(pkgReg, installers.NewRegistry(), stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"nonexistent"}, &mockSpinner{})
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	svc := newTestRemoveSvc(pkgReg, installers.NewRegistry(), stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"testpkg"}, &mockSpinner{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveWithInstaller(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"testpkg": {Type: "apt"},
	}}
	stateSvc.Save(st)

	mockInst := &mockInstaller{removeErr: nil}
	instReg.Register(pkg.TypeApt, mockInst)

	svc := newTestRemoveSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"testpkg"}, &mockSpinner{})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	st, _ = stateSvc.Load()
	if stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be removed from state")
	}
	if len(mockInst.removed) != 1 || mockInst.removed[0] != "testpkg" {
		t.Fatalf("expected installer.Remove to be called with testpkg, got %v", mockInst.removed)
	}
}

func TestRemoveMultiple(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg1", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg2", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"pkg1": {Type: "apt"},
		"pkg2": {Type: "apt"},
	}}
	stateSvc.Save(st)

	mockInst := &mockInstaller{}
	instReg.Register(pkg.TypeApt, mockInst)

	svc := newTestRemoveSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"pkg1", "pkg2"}, &mockSpinner{})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if len(mockInst.removed) != 2 {
		t.Fatalf("expected 2 removals, got %d", len(mockInst.removed))
	}
}

func TestRemoveForceReinstalls(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"testpkg": {Type: "apt"},
	}}
	stateSvc.Save(st)

	mockInst := &mockInstaller{}
	instReg.Register(pkg.TypeApt, mockInst)

	svc := newTestRemoveSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"testpkg"}, &mockSpinner{})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if len(mockInst.removed) != 1 {
		t.Fatalf("expected 1 removal, got %d", len(mockInst.removed))
	}
}

func TestRemoveForceRemovesAlreadyRemoved(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	mockInst := &mockInstaller{}
	instReg.Register(pkg.TypeApt, mockInst)

	svc := newTestRemoveSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Remove(ctx, []string{"testpkg"}, &mockSpinner{})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
}
