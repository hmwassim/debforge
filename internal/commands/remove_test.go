package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/coreservices"
	"github.com/hmwassim/debforge/internal/ports"
)

type mockRemoveService struct {
	removeErr  error
	called     bool
	calledArgs []string
}

func (m *mockRemoveService) Remove(ctx context.Context, pkgNames []string, spinner ports.Spinner) error {
	m.called = true
	m.calledArgs = pkgNames
	return m.removeErr
}

var _ services.PackageRemover = (*mockRemoveService)(nil)

func TestRemoveCommandNoArgs(t *testing.T) {
	svc := &mockRemoveService{}
	pkgReg := pkg.NewRegistry()
	cmd := NewRemoveCommand(svc, pkgReg, &mockUI{})
	ctx := context.Background()

	err := cmd.Run(ctx, []string{})
	if err == nil {
		t.Fatal("expected error for no package name")
	}
}

func TestRemoveCommandUnknownPackage(t *testing.T) {
	svc := &mockRemoveService{}
	pkgReg := pkg.NewRegistry()
	cmd := NewRemoveCommand(svc, pkgReg, &mockUI{})
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
}

func TestRemoveCommandKnownPackage(t *testing.T) {
	svc := &mockRemoveService{removeErr: nil}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewRemoveCommand(svc, pkgReg, &mockUI{})
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !svc.called {
		t.Fatal("Remove not called")
	}
	if len(svc.calledArgs) != 1 || svc.calledArgs[0] != "testpkg" {
		t.Fatalf("expected [testpkg], got %v", svc.calledArgs)
	}
}

func TestRemoveCommandMultiplePackages(t *testing.T) {
	svc := &mockRemoveService{removeErr: nil}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg1", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg2", Type: pkg.TypeDeb}})
	cmd := NewRemoveCommand(svc, pkgReg, &mockUI{})
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"pkg1", "pkg2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !svc.called {
		t.Fatal("Remove not called")
	}
	if len(svc.calledArgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(svc.calledArgs))
	}
}

func TestRemoveCommandServiceError(t *testing.T) {
	svc := &mockRemoveService{removeErr: errors.New("remove failed")}
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "testpkg", Type: pkg.TypeApt}})
	cmd := NewRemoveCommand(svc, pkgReg, &mockUI{})
	ctx := context.Background()

	err := cmd.Run(ctx, []string{"testpkg"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "remove failed" {
		t.Fatalf("expected 'remove failed', got %v", err)
	}
}
