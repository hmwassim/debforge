package service

import (
	"context"
	"fmt"
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

	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed", nil)
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

	_, err := svc.processOne(ctx, "root", false, true, st, spinner, "install", "installed", nil)
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
	_, err := svc.processOne(ctx, "root", true, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if len(st.Packages) != 2 {
		t.Fatalf("expected 2 packages in state, got %d", len(st.Packages))
	}

}

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

	// countInstallCalls records every Install call.
	type callCountRecorder struct {
		variantRecorder
	}
	recorder := &callCountRecorder{}

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

	// Provide the fields checkInstalled needs (it calls runner.Run for dpkg).
	// Use a runner that reports nothing installed so the system-installed
	// check never short-circuits.
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	cleanup := func() { os.Remove(tmpFile.Name()) }
	return svc, &recorder.variantRecorder, cleanup
}

type nopRunner struct{}

func (n *nopRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}
func (n *nopRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}

type successRunner struct{}

func (r *successRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
}

func (r *successRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
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

	// 3 calls: root-a, shared, root-b  (not 4: root-a, shared, root-b, shared)
	if len(recorder.forceFlags) != 3 {
		t.Fatalf("expected 3 install calls (root-a + shared + root-b), got %d", len(recorder.forceFlags))
	}

	// shared should be in state
	entry, ok := st.Packages["shared"]
	if !ok {
		t.Fatal("expected shared to be in state")
	}
	if entry.Type != "apt" {
		t.Errorf("expected shared type 'apt', got %q", entry.Type)
	}
}

type failAfterRecorder struct {
	variantRecorder
	failAfter int
	callCount int
}

func (r *failAfterRecorder) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	r.callCount++
	if r.callCount >= r.failAfter {
		return fmt.Errorf("simulated failure on %s", p.Name)
	}
	return r.variantRecorder.Install(ctx, p, spinner)
}

// setupPersistenceTest creates an InstallService with a single apt package
// and a real state file on disk.
func setupPersistenceTest(t *testing.T) (*InstallService, string, func()) {
	t.Helper()

	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
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
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	cleanup := func() { os.Remove(tmpFile.Name()) }
	return svc, tmpFile.Name(), cleanup
}

func TestProcessOne_successPersistsState(t *testing.T) {
	svc, statePath, cleanup := setupPersistenceTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	_, err := svc.processOne(ctx, "test-pkg", false, true, st, spinner, "install", "installed", nil)
	if err != nil {
		t.Fatalf("processOne: %v", err)
	}

	entry, ok := st.Packages["test-pkg"]
	if !ok {
		t.Fatal("expected test-pkg in in-memory state")
	}
	if entry.Type != "apt" {
		t.Errorf("expected type 'apt', got %q", entry.Type)
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	diskEntry, ok := loaded.Packages["test-pkg"]
	if !ok {
		t.Fatal("expected test-pkg in persisted state on disk")
	}
	if diskEntry.Type != "apt" {
		t.Errorf("expected persisted type 'apt', got %q", diskEntry.Type)
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

	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)

	svc := &InstallService{
		reg:      reg,
		instReg:  instReg,
		resolver: NewResolver(reg),
		state:    stateSvc,
	}
	svc.runner = &nopRunner{}
	svc.fs = fs.NewFileSystem()

	st := &State{Packages: map[string]PkgEntry{}}
	ctx := context.Background()
	spinner := &mockSpinner{}

	err = svc.processAll(ctx, []string{"root-a", "root-b"}, false, true, st, spinner, "install", "installed")
	if err == nil {
		t.Fatal("expected error from processAll due to root-b failure")
	}

	if _, ok := st.Packages["root-a"]; !ok {
		t.Error("expected root-a in in-memory state after partial failure")
	}
	if _, ok := st.Packages["root-b"]; ok {
		t.Error("unexpected root-b in in-memory state after failure")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
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

func TestCheckInstalled_doesNotPersist(t *testing.T) {
	svc, statePath, cleanup := setupPersistenceTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}

	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}
	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := checkInstalled(ctx, svc.state, st, "test-pkg", svc.runner, svc.fs, p, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled from checkInstalled")
	}
	if !cleanedUp {
		t.Error("expected cleanedUp=true for stale entry")
	}

	if _, ok := st.Packages["test-pkg"]; ok {
		t.Error("expected test-pkg removed from in-memory state by checkInstalled")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in persisted state (checkInstalled must not persist)")
	}
}

func TestCheckInstalled_installed(t *testing.T) {
	svc, statePath, cleanup := setupPersistenceTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{
		"test-pkg": {Type: "apt"},
	}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}

	svc.runner = &successRunner{}

	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := checkInstalled(ctx, svc.state, st, "test-pkg", svc.runner, svc.fs, p, spinner)
	if err != nil {
		t.Fatalf("expected no error for installed package, got: %v", err)
	}
	if cleanedUp {
		t.Error("expected cleanedUp=false for installed package")
	}
	if _, ok := st.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in in-memory state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, ok := loaded.Packages["test-pkg"]; !ok {
		t.Error("expected test-pkg still in persisted state")
	}
}

func TestCheckInstalled_notInstalled(t *testing.T) {
	svc, statePath, cleanup := setupPersistenceTest(t)
	defer cleanup()

	st := &State{Packages: map[string]PkgEntry{}}
	if err := svc.state.Save(st); err != nil {
		t.Fatalf("save initial state: %v", err)
	}

	p, ok := svc.reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("lookup test-pkg")
	}
	ctx := context.Background()
	spinner := &mockSpinner{}

	cleanedUp, err := checkInstalled(ctx, svc.state, st, "test-pkg", svc.runner, svc.fs, p, spinner)
	if err == nil {
		t.Fatal("expected ErrNotInstalled for missing package")
	}
	if cleanedUp {
		t.Error("expected cleanedUp=false for package not in state")
	}

	diskStore := store.NewStore[State](fs.NewFileSystem(), statePath)
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if len(loaded.Packages) != 0 {
		t.Error("expected persisted state to remain empty")
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
