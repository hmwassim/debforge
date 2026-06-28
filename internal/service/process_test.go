package service

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func setupVariantTest(t *testing.T) (*InstallService, *variantRecorder, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}, "staging": {"pkg-staging"}},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	return svc, recorder, cleanup
}

func TestProcessOne_variant_firstInstall(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if len(recorder.variants) != 1 {
		t.Fatalf("expected 1 install call, got %d", len(recorder.variants))
	}
	if recorder.variants[0] != "" {
		t.Errorf("expected empty variant on first install, got %q", recorder.variants[0])
	}
}

func TestProcessOne_variant_reinstall(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Variant: "staging"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if len(recorder.variants) != 1 {
		t.Fatalf("expected 1 install call, got %d", len(recorder.variants))
	}
	if recorder.variants[0] != "staging" {
		t.Errorf("expected variant %q, got %q", "staging", recorder.variants[0])
	}
}

func TestProcessOne_variant_update(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Variant: "stable"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "update", "updated", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if len(recorder.variants) != 1 {
		t.Fatalf("expected 1 install call, got %d", len(recorder.variants))
	}
	if recorder.variants[0] != "stable" {
		t.Errorf("expected variant %q, got %q", "stable", recorder.variants[0])
	}
}

// setupDepTest creates a service with a two-package tree:
//
//	root (apt) depends on dep (apt).
func setupDepTest(t *testing.T) (*InstallService, *variantRecorder, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "root",
		Type:    pkg.TypeApt,
		Depends: []string{"dep"},
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}, "staging": {"pkg-staging"}},
		},
	})
	reg.Register(&pkg.Package{
		Name: "dep",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"default": {"pkg-default"}},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	stateSvc, _, cleanup := newStateManagerForTest(t)

	svc := &InstallService{
		baseService: baseService{reg: reg, instReg: instReg, state: stateSvc},
		resolver:    NewResolver(reg),
	}

	return svc, recorder, cleanup
}

func TestProcessOne_variant_switching(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Variant: "staging"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if len(recorder.variants) != 1 {
		t.Fatalf("expected 1 install call, got %d", len(recorder.variants))
	}
	if recorder.variants[0] != "staging" {
		t.Errorf("expected variant %q, got %q", "staging", recorder.variants[0])
	}
}
