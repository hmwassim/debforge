package main

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestInstall_success(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var promptCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { promptCalled = true; return true },
	}

	code := h.install(context.Background(), u, []string{"test-pkg"}, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !promptCalled {
		t.Error("expected prompt to be called")
	}
}

func TestInstall_error(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{installErr: errors.New("install failed")})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.install(context.Background(), u, []string{"test-pkg"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

func TestInstall_selectVariantsError(t *testing.T) {
	reg := pkg.NewRegistry()
	// Package with a dependency that does not exist — causes SelectVariants
	// to fail during Resolve.
	reg.Register(&pkg.Package{
		Name:    "test-pkg",
		Type:    pkg.TypeApt,
		Depends: []string{"nonexistent-dep"},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.install(context.Background(), u, []string{"test-pkg"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

func TestInstall_gpuCheckSuccess(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"lspci": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return []byte("NVIDIA Corporation GA104 [GeForce RTX 3070]"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
	}

	code := h.install(context.Background(), u, []string{"nvidia"}, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestInstall_gpuCheckFail(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"lspci": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return []byte("Intel Corporation Device"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var warnCalled bool
	u := &testutil.MockUI{
		WarnFunc: func(_ string, _ ...any) { warnCalled = true },
	}

	code := h.install(context.Background(), u, []string{"nvidia"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !warnCalled {
		t.Error("expected u.Warn to be called for missing GPU")
	}
}

func TestInstall_conflictCheck(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "test-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{Conflicts: []string{"conflict-pkg"}},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				return []byte("installed\n"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var infoCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		InfoFunc:   func(_ string, _ ...any) { infoCalled = true },
	}

	code := h.install(context.Background(), u, []string{"test-pkg"}, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !infoCalled {
		t.Error("expected u.Info to be called for conflicts")
	}
}

// ---- GPU check tests -------------------------------------------------------

func TestInstall_GPUCheck_nvidiaDepPasses(t *testing.T) {
	reg := pkg.NewRegistry()
	nv := &pkg.Package{Name: "nvflux", Type: pkg.TypeSource, Depends: []string{"nvidia"}}
	reg.Register(nv)
	reg.Register(&pkg.Package{Name: "nvidia", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeSource, &mockInstaller{})
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)
	cfg := &self.Config{LockPath: "/lock"}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "lspci" {
				return []byte("NVIDIA GeForce RTX 4090"), nil, nil
			}
			return nil, nil, nil
		},
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
	}
	code := h.install(context.Background(), u, []string{"nvflux"}, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestInstall_GPUCheck_nvidiaDepFails(t *testing.T) {
	reg := pkg.NewRegistry()
	nv := &pkg.Package{Name: "nvflux", Type: pkg.TypeSource, Depends: []string{"nvidia"}}
	reg.Register(nv)
	reg.Register(&pkg.Package{Name: "nvidia", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeSource, &mockInstaller{})
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)
	cfg := &self.Config{LockPath: "/lock"}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "lspci" {
				return []byte("Intel integrated graphics"), nil, nil
			}
			return nil, nil, nil
		},
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var warnCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		WarnFunc:   func(_ string, _ ...any) { warnCalled = true },
	}
	code := h.install(context.Background(), u, []string{"nvflux"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !warnCalled {
		t.Error("expected Warn to be called when GPU check fails")
	}
}

func TestInstall_GPUCheck_noNvidiaDepSkipsCheck(t *testing.T) {
	reg := pkg.NewRegistry()
	testPkg := &pkg.Package{Name: "firefox", Type: pkg.TypeApt}
	reg.Register(testPkg)

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)
	cfg := &self.Config{LockPath: "/lock"}

	lspciCalled := false
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "lspci" {
				lspciCalled = true
			}
			return nil, nil, nil
		},
	}

	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
	}
	code := h.install(context.Background(), u, []string{"firefox"}, false)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if lspciCalled {
		t.Error("lspci should not be called for unrelated package")
	}
}
