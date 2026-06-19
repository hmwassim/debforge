package services

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/dependency"
	"github.com/hmwassim/debforge/internal/services/state"
)

type lifecycleInstaller struct {
	installs []string
	removes  []string
}

func (l *lifecycleInstaller) Install(ctx context.Context, p *pkg.Package) error {
	l.installs = append(l.installs, p.Name)
	return nil
}

func (l *lifecycleInstaller) Remove(ctx context.Context, p *pkg.Package) error {
	l.removes = append(l.removes, p.Name)
	return nil
}

func (l *lifecycleInstaller) Update(ctx context.Context, p *pkg.Package) error {
	l.installs = append(l.installs, p.Name)
	return nil
}

func TestLifecycleInstallRemove(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	inst := &lifecycleInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	installSvc := NewInstallService(pkgReg, instReg, stateSvc, dependency.NewResolver(pkgReg), &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	removeSvc := NewRemoveService(pkgReg, instReg, stateSvc, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	if err := installSvc.Install(ctx, []string{"testpkg"}, nil, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be installed after install")
	}
	if len(inst.installs) != 1 || inst.installs[0] != "testpkg" {
		t.Fatalf("expected 1 install of testpkg, got %v", inst.installs)
	}

	if err := removeSvc.Remove(ctx, []string{"testpkg"}); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	st, _ = stateSvc.Load()
	if stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be removed after remove")
	}
	if len(inst.removes) != 1 || inst.removes[0] != "testpkg" {
		t.Fatalf("expected 1 remove of testpkg, got %v", inst.removes)
	}
}

func TestLifecycleInstallRemoveReinstall(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	inst := &lifecycleInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	installSvc := NewInstallService(pkgReg, instReg, stateSvc, dependency.NewResolver(pkgReg), &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	removeSvc := NewRemoveService(pkgReg, instReg, stateSvc, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	installSvc.Install(ctx, []string{"testpkg"}, nil, false)
	removeSvc.Remove(ctx, []string{"testpkg"})

	if err := installSvc.Install(ctx, []string{"testpkg"}, nil, false); err != nil {
		t.Fatalf("reinstall after remove failed: %v", err)
	}
	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be installed after reinstall")
	}
	if len(inst.installs) != 2 {
		t.Fatalf("expected 2 total installs, got %d", len(inst.installs))
	}
}

func TestLifecycleInstallForceReinstall(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	inst := &lifecycleInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	installSvc := NewInstallService(pkgReg, instReg, stateSvc, dependency.NewResolver(pkgReg), &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	installSvc.Install(ctx, []string{"testpkg"}, nil, false)
	if len(inst.installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(inst.installs))
	}

	if err := installSvc.Install(ctx, []string{"testpkg"}, nil, true); err != nil {
		t.Fatalf("force reinstall failed: %v", err)
	}
	if len(inst.installs) != 2 {
		t.Fatalf("expected 2 installs with force, got %d", len(inst.installs))
	}
	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to remain installed after force reinstall")
	}
}

func TestLifecycleInstallRemoveWithDeps(t *testing.T) {
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

	inst := &lifecycleInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	resolver := dependency.NewResolver(pkgReg)
	installSvc := NewInstallService(pkgReg, instReg, stateSvc, resolver, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	removeSvc := NewRemoveService(pkgReg, instReg, stateSvc, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	if err := installSvc.Install(ctx, []string{"main"}, nil, false); err != nil {
		t.Fatalf("install with deps failed: %v", err)
	}
	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "main") || !stateSvc.IsInstalled(st, "dep") {
		t.Fatal("expected both main and dep to be installed")
	}

	if err := removeSvc.Remove(ctx, []string{"main"}); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	st, _ = stateSvc.Load()
	if stateSvc.IsInstalled(st, "main") {
		t.Fatal("expected main to be removed")
	}
	if !stateSvc.IsInstalled(st, "dep") {
		t.Fatal("expected dep to remain installed after removing main")
	}
	if len(inst.removes) != 1 || inst.removes[0] != "main" {
		t.Fatalf("expected only main to be removed, got %v", inst.removes)
	}
}

func TestLifecycleRemoveNonExistentThenInstall(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	instReg := installers.NewRegistry()

	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})

	inst := &lifecycleInstaller{}
	instReg.Register(pkg.TypeApt, inst)

	installSvc := NewInstallService(pkgReg, instReg, stateSvc, dependency.NewResolver(pkgReg), &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	removeSvc := NewRemoveService(pkgReg, instReg, stateSvc, &mockUI{}, &mockLocker{}, "/tmp/test.lock")
	ctx := context.Background()

	if err := removeSvc.Remove(ctx, []string{"testpkg"}); err != nil {
		t.Fatalf("remove of non-installed should not error: %v", err)
	}
	if len(inst.removes) != 0 {
		t.Fatalf("expected no removes for non-installed package, got %v", inst.removes)
	}

	if err := installSvc.Install(ctx, []string{"testpkg"}, nil, false); err != nil {
		t.Fatalf("install after non-existent remove failed: %v", err)
	}
	st, _ := stateSvc.Load()
	if !stateSvc.IsInstalled(st, "testpkg") {
		t.Fatal("expected testpkg to be installed after lifecycle")
	}
}
