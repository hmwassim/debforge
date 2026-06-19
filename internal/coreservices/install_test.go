package services

import (
	"context"
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/dependency"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/statestore"
)

func newTestInstallSvc(pkgReg *pkg.Registry, instReg *installers.Registry, stateSvc *state.Service) *InstallService {
	return NewInstallService(pkgReg, instReg, stateSvc, dependency.NewResolver(pkgReg), &mockUI{}, &mockLocker{}, "/tmp/test.lock")
}

func TestInstallUnknownPackage(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	stateSvc := state.NewService(newMemFS(), "/tmp/states")
	svc := newTestInstallSvc(pkgReg, installers.NewRegistry(), stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"nonexistent"}, nil, false)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestInstallAlreadyInstalled(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"testpkg": {Type: "apt"},
	}}
	stateSvc.Save(st)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"testpkg"}, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type memInstaller struct {
	installed []string
}

func (m *memInstaller) Install(ctx context.Context, p *pkg.Package) error {
	m.installed = append(m.installed, p.Name)
	return nil
}

func (m *memInstaller) Remove(ctx context.Context, p *pkg.Package) error {
	return nil
}

func (m *memInstaller) Update(ctx context.Context, p *pkg.Package) error {
	return m.Install(ctx, p)
}

func TestInstallWithInstaller(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	memInst := &memInstaller{}
	instReg.Register(pkg.TypeApt, memInst)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"testpkg"}, nil, false)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	st, err := stateSvc.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be installed in state")
	}
	if len(memInst.installed) != 1 || memInst.installed[0] != "testpkg" {
		t.Fatal("expected testpkg to be installed via installer")
	}
}

type trackingInstaller struct {
	installed []string
}

func (m *trackingInstaller) Install(ctx context.Context, p *pkg.Package) error {
	m.installed = append(m.installed, p.Name)
	return nil
}

func (m *trackingInstaller) Remove(ctx context.Context, p *pkg.Package) error {
	return nil
}

func (m *trackingInstaller) Update(ctx context.Context, p *pkg.Package) error {
	return m.Install(ctx, p)
}

func TestInstallWithDep(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	depPkg := &pkg.Package{Metadata: pkg.Metadata{Name: "dep", Type: pkg.TypeApt}}
	mainPkg := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "main", Type: pkg.TypeApt},
		InstallSpec: pkg.InstallSpec{Depends: []string{"dep"}},
	}
	pkgReg.Register(depPkg)
	pkgReg.Register(mainPkg)

	inst := &trackingInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"main"}, nil, false)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "main") || !stateSvc.IsInstalled(st, "dep") {
		t.Fatal("expected both main and dep to be installed")
	}
	if len(inst.installed) != 2 {
		t.Fatalf("expected 2 installs, got %d", len(inst.installed))
	}
}

type errLocker struct{}

func (m *errLocker) Acquire(ctx context.Context, name string) (func(), error) {
	return nil, os.ErrPermission
}

func TestInstallLockError(t *testing.T) {
	svc := NewInstallService(nil, nil, nil, nil, &mockUI{}, &errLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	err := svc.Install(ctx, []string{"pkg"}, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInstallForceReinstalls(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"testpkg": {Type: "apt"},
	}}
	stateSvc.Save(st)

	memInst := &memInstaller{}
	instReg.Register(pkg.TypeApt, memInst)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"testpkg"}, nil, true)
	if err != nil {
		t.Fatalf("install with force failed: %v", err)
	}

	if len(memInst.installed) != 1 {
		t.Fatalf("expected 1 install with force, got %d", len(memInst.installed))
	}
}

func TestInstallForceFalseSkipsInstalled(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"testpkg": {Type: "apt"},
	}}
	stateSvc.Save(st)

	memInst := &memInstaller{}
	instReg.Register(pkg.TypeApt, memInst)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"testpkg"}, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(memInst.installed) != 0 {
		t.Fatalf("expected 0 installs without force, got %d", len(memInst.installed))
	}
}

type errFS struct {
	*memFS
}

func (f *errFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error {
	return os.ErrPermission
}

func TestInstallRollbackOnStateSaveFailure(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := &errFS{newMemFS()}
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	inst := &trackingInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	svc := newTestInstallSvc(pkgReg, instReg, stateSvc)
	ctx := context.Background()

	err := svc.Install(ctx, []string{"testpkg"}, nil, false)
	if err == nil {
		t.Fatal("expected error due to state save failure")
	}
	if len(inst.installed) != 1 || inst.installed[0] != "testpkg" {
		t.Fatal("expected testpkg to be installed before rollback")
	}
	st, _ := stateSvc.Load()
	if stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to NOT be in state after rollback (state save failed)")
	}
}
