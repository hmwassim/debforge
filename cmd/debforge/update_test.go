package main

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestUpdate_success(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"test-pkg":{"type":"apt"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"apt-get": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return nil, nil, nil
			},
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return []byte("installed\n"), nil, nil
			},
		},
	}

	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, aptArgs []string, _ ports.Spinner) error {
		t.Fatal("RunUpgrade should not be called when allMode=false")
		return nil
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var promptCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { promptCalled = true; return true },
	}

	code := h.update(context.Background(), u, []string{"test-pkg"}, false, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !promptCalled {
		t.Error("expected prompt to be called")
	}
}

func TestUpdate_runUpdateError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"test-pkg":{"type":"apt"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"apt-get": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return nil, nil, errors.New("apt-get update failed")
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.update(context.Background(), u, []string{"test-pkg"}, false, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

func TestUpdate_allMode(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"test-pkg":{"type":"apt"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"apt-get": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return nil, nil, nil
			},
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return []byte("installed\n"), nil, nil
			},
		},
	}

	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })

	var upgradeCalled bool
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, aptArgs []string, _ ports.Spinner) error {
		upgradeCalled = true
		if len(aptArgs) < 2 || aptArgs[0] != "full-upgrade" || aptArgs[1] != "-y" {
			t.Errorf("unexpected aptArgs: %v", aptArgs)
		}
		return nil
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var promptCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { promptCalled = true; return true },
	}

	code := h.update(context.Background(), u, []string{"test-pkg"}, false, true)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !promptCalled {
		t.Error("expected prompt to be called")
	}
	if !upgradeCalled {
		t.Error("expected RunUpgrade to be called")
	}
}

func TestUpdate_allMode_runUpgradeError(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"test-pkg":{"type":"apt"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"apt-get": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return nil, nil, nil
			},
		},
	}

	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })

	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return errors.New("full-upgrade failed")
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.update(context.Background(), u, []string{"test-pkg"}, false, true)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}
