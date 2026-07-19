package service

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

// Constructor tests

func TestNewInstallService(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := NewInstallService(reg, instReg, NewResolver(reg), stateSvc, nil, "", nil, nil, nil, nopAptUpdater{}, nopExtrepoManager{}, nopPackageLister{})
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewRemoveService(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _, cleanup := newStateManagerForTest(t)
	defer cleanup()

	svc := NewRemoveService(reg, instReg, stateSvc, nil, "", nil, nil, nil, nopAptUpdater{}, nopExtrepoManager{}, nopPackageLister{})
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

func TestLookupInstaller_unregisteredType(t *testing.T) {
	instReg := installer.NewRegistry()
	_, err := LookupInstaller(instReg, pkg.TypeApt)
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
	if !contains(err.Error(), "no installer for type") {
		t.Errorf("expected 'no installer for type', got: %v", err)
	}
}

// mockVariantSelector is used by SelectVariants tests in install_test.go.

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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
