package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type mockInstallService struct {
	installErr error
	called     bool
	calledArgs []string
}

func (m *mockInstallService) Install(ctx context.Context, pkgNames []string, variants map[string]string, force bool) error {
	m.called = true
	m.calledArgs = pkgNames
	return m.installErr
}

var _ services.PackageInstaller = (*mockInstallService)(nil)

func newMockUI() *mockUI {
	return &mockUI{}
}

func TestInstallCommandNoArgs(t *testing.T) {
	svc := &mockInstallService{}
	cmd := NewInstallCommand(svc, pkg.NewRegistry(), newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{})
	if err == nil {
		t.Fatal("expected error for no package name")
	}
}

func TestInstallCommandUnknownPackage(t *testing.T) {
	svc := &mockInstallService{}
	pkgReg := pkg.NewRegistry()
	cmd := NewInstallCommand(svc, pkgReg, newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestInstallCommandKnownPackage(t *testing.T) {
	svc := &mockInstallService{installErr: nil}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewInstallCommand(svc, pkgReg, newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !svc.called {
		t.Fatal("Install not called")
	}
	if len(svc.calledArgs) != 1 || svc.calledArgs[0] != "testpkg" {
		t.Fatalf("expected [testpkg], got %v", svc.calledArgs)
	}
}

func TestInstallCommandForceFlag(t *testing.T) {
	svc := &mockInstallService{installErr: nil}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewInstallCommand(svc, pkgReg, newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg", "-f"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallCommandForceFlagLong(t *testing.T) {
	svc := &mockInstallService{installErr: nil}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewInstallCommand(svc, pkgReg, newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg", "--force"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallCommandServiceError(t *testing.T) {
	svc := &mockInstallService{installErr: errors.New("install failed")}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewInstallCommand(svc, pkgReg, newMockUI())
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "install failed" {
		t.Fatalf("expected 'install failed', got %v", err)
	}
}
