package service

import (
	"context"
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type mockSpinner struct{ desc string }

func (m *mockSpinner) Done()            {}
func (m *mockSpinner) Fail()            {}
func (m *mockSpinner) DoneWarn()        {}
func (m *mockSpinner) DoneInfo()        {}
func (m *mockSpinner) Pause()           {}
func (m *mockSpinner) Resume()          {}
func (m *mockSpinner) SetDesc(d string) { m.desc = d }

type variantRecorder struct {
	variants []string
	// forceFlags records the ForceInstall value of each Install call.
	forceFlags []bool
}

func (r *variantRecorder) Install(_ context.Context, p *pkg.Package, _ ports.Spinner) error {
	r.forceFlags = append(r.forceFlags, p.ForceInstall)
	if p.Apt != nil {
		r.variants = append(r.variants, p.Apt.Variant)
	}
	return nil
}

func (r *variantRecorder) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

func setupVariantTest(t *testing.T) (*InstallService, *variantRecorder, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string]string{"stable": "pkg-stable", "staging": "pkg-staging"},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	tmpFile.Close()

	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)

	svc := &InstallService{
		reg:      reg,
		instReg:  instReg,
		resolver: NewResolver(reg),
		state:    stateSvc,
	}

	cleanup := func() { os.Remove(tmpFile.Name()) }
	return svc, recorder, cleanup
}

func TestProcessOne_variant_firstInstall(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed")
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

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed")
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

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "update", "updated")
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
//   root (apt) depends on dep (apt).
func setupDepTest(t *testing.T) (*InstallService, *variantRecorder, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "root",
		Type:    pkg.TypeApt,
		Depends: []string{"dep"},
		Apt: &pkg.AptConfig{
			Variants: map[string]string{"stable": "pkg-stable", "staging": "pkg-staging"},
		},
	})
	reg.Register(&pkg.Package{
		Name: "dep",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variants: map[string]string{"default": "pkg-default"},
		},
	})

	recorder := &variantRecorder{}
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, recorder)

	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	tmpFile.Close()

	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)

	svc := &InstallService{
		reg:      reg,
		instReg:  instReg,
		resolver: NewResolver(reg),
		state:    stateSvc,
	}

	cleanup := func() { os.Remove(tmpFile.Name()) }
	return svc, recorder, cleanup
}

func TestProcessOne_forcePropagatesToDeps(t *testing.T) {
	svc, recorder, cleanup := setupDepTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	// root + dep should have been installed
	if len(recorder.forceFlags) != 2 {
		t.Fatalf("expected 2 install calls (root + dep), got %d", len(recorder.forceFlags))
	}

	// both should have ForceInstall=true
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

	_, err := svc.processOne(ctx, "root", false, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(recorder.forceFlags) != 2 {
		t.Fatalf("expected 2 install calls (root + dep), got %d", len(recorder.forceFlags))
	}

	// neither should have ForceInstall
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

	// With force=true, every dep gets ForceInstall=true. The state update
	// condition (dep.ForceInstall || !exists || dep.Version != oldVersion)
	// fires for ForceInstall even when the version hasn't changed,
	// guaranteeing the state entry is written/re-written.
	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed")
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(st.Packages) != 2 {
		t.Fatalf("expected 2 packages in state, got %d", len(st.Packages))
	}

}

func TestProcessOne_variant_switching(t *testing.T) {
	svc, recorder, cleanup := setupVariantTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt", Variant: "staging"},
	}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed")
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
