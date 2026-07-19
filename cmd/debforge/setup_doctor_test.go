package main

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/setup"
	"github.com/hmwassim/debforge/internal/testutil"
)

// mockStep returns a Step that returns the given CheckResult and Apply error.
type mockStep struct {
	name  string
	check setup.CheckResult
	apply error
}

func (s *mockStep) Name() string                                                { return s.name }
func (s *mockStep) Check(_ context.Context, _ *setup.Context) setup.CheckResult { return s.check }
func (s *mockStep) Apply(_ context.Context, _ *setup.Context, _ setup.CheckResult) error {
	return s.apply
}

func newSetupHandler(t *testing.T, sys ports.System, cfg *self.Config, fsys *testutil.MockFileSystem, runner ports.CommandRunner) *commandHandler {
	t.Helper()
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})
	instReg.Register(pkg.TypeDeb, &mockInstaller{})
	instReg.Register(pkg.TypeSource, &mockInstaller{})
	instReg.Register(pkg.TypeConfig, &mockInstaller{})
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil {
		if err := stateSvc.Save(&service.State{Packages: make(map[string]service.PkgEntry)}); err != nil {
			t.Fatalf("save state: %v", err)
		}
	}
	return &commandHandler{
		reg: reg, instReg: instReg, stateSvc: stateSvc,
		locker: &testutil.MockLocker{}, cfg: cfg, runner: runner, fsys: fsys, sys: sys,
		aptUpd: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}, pkgList: testutil.NopPackageLister{},
	}
}

// ---- setup tests -----------------------------------------------------------

func TestSetup_notPrivileged(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{}, &self.Config{}, fsys, nil)
	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}
	code := h.setup(context.Background(), u, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected Error to be called for not privileged")
	}
}

func TestSetup_promptCancelled(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return []byte("installed\n"), nil, nil
		},
	})
	var infoCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return false },
		InfoFunc:   func(_ string, _ ...any) { infoCalled = true },
	}
	code := h.setup(context.Background(), u, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !infoCalled {
		t.Error("expected Info to be called with cancellation message")
	}
}

// ---- doctor tests ---------------------------------------------------------

func TestDoctor_notPrivileged(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{}, &self.Config{}, fsys, nil)
	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}
	code := h.doctor(context.Background(), u)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected Error to be called")
	}
}

func TestDoctor_allMissing(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{
		SetupStatePath: "/setup_state.json",
	}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var infoCount int
	u := &testutil.MockUI{
		InfoFunc: func(_ string, _ ...any) { infoCount++ },
	}
	code := h.doctor(context.Background(), u)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if infoCount == 0 {
		t.Error("expected Info to be called for missing steps")
	}
}

func restoreDefaults() {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{
			&setup.ReposStep{Sources: []setup.ConfigFile{{Path: "/etc/apt/sources.list", Content: setup.DebianSourcesList}}},
			&setup.I386Step{},
			&setup.UpgradeStep{},
			&setup.FirmwareStep{},
			&setup.DevtoolsStep{},
			&setup.KernelStep{},
			&setup.ZramStep{},
			&setup.ResolvedStep{},
			&setup.TimesyncdStep{},
			&setup.ExtrepoStep{},
			&setup.MesaStep{},
			&setup.MultimediaStep{},
			&setup.FontsStep{},
			&setup.DesktopStep{},
		}
	}
}

func TestSetup_forceMode(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "test", check: setup.CheckResult{Status: setup.StatusSatisfied}, apply: nil}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
	}
	code := h.setup(context.Background(), u, true)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestSetup_saveStateError(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "test", check: setup.CheckResult{Status: setup.StatusSatisfied}, apply: nil}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	fsys.WriteFileFunc = func(_ string, _ []byte, _ int) error {
		return errors.New("write error")
	}
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}
	code := h.setup(context.Background(), u, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected Error to be called for save state error")
	}
}

func TestSetup_runnerRunError(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "test", check: setup.CheckResult{Status: setup.StatusMissing}, apply: errors.New("apply failed")}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}
	code := h.setup(context.Background(), u, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected Error to be called for runner.Run error")
	}
}

// ---- doctor tests ---------------------------------------------------------

func TestDoctor_allSatisfied(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "test-step", check: setup.CheckResult{Status: setup.StatusSatisfied}}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var successCount int
	u := &testutil.MockUI{
		SuccessFunc: func(_ string, _ ...any) { successCount++ },
	}
	code := h.doctor(context.Background(), u)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if successCount == 0 {
		t.Error("expected at least one Success call for satisfied steps")
	}
}

func TestDoctor_driftedStep(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "drifted", check: setup.CheckResult{Status: setup.StatusDrifted, Summary: "user modified"}}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var warnCalled bool
	u := &testutil.MockUI{
		WarnFunc: func(_ string, _ ...any) { warnCalled = true },
	}
	code := h.doctor(context.Background(), u)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !warnCalled {
		t.Error("expected Warn to be called for drifted step")
	}
}

func TestDoctor_conflictStep(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "conflict", check: setup.CheckResult{Status: setup.StatusConflict, Summary: "conflict detected"}}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var warnCalled bool
	u := &testutil.MockUI{
		WarnFunc: func(_ string, _ ...any) { warnCalled = true },
	}
	code := h.doctor(context.Background(), u)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !warnCalled {
		t.Error("expected Warn to be called for conflict step")
	}
}

func TestDoctor_errorStep(t *testing.T) {
	setup.DefaultSteps = func() []setup.Step {
		return []setup.Step{&mockStep{name: "error-step", check: setup.CheckResult{Status: setup.StatusError, Summary: "check failed"}}}
	}
	defer restoreDefaults()

	fsys := testutil.NewMockFileSystem()
	h := newSetupHandler(t, &testutil.MockSystem{Privileged: true}, &self.Config{SetupStatePath: "/state.json"}, fsys, &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	})
	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}
	code := h.doctor(context.Background(), u)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected Error to be called for error step")
	}
}

func TestDoctor_loadStateError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) {
		return false, errors.New("stat error")
	}
	h := &commandHandler{
		fsys: fsys,
		sys:  &testutil.MockSystem{Privileged: true},
		cfg:  &self.Config{SetupStatePath: "/setup_state.json"},
		runner: &testutil.MockRunner{
			RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
				return nil, nil, nil
			},
		},
		aptUpd: testutil.NopAptUpdater{}, extrepo: testutil.NopExtrepoManager{}, pkgList: testutil.NopPackageLister{},
	}
	// LoadState swallows errors, so doctor proceeds to CheckAll.
	// Verify it runs without panic and returns the expected result.
	code := h.doctor(context.Background(), &testutil.MockUI{})
	if code != 1 {
		t.Errorf("expected 1 (all missing), got %d", code)
	}
}
