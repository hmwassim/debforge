package self

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

// removerTestDeps holds the dependencies created for a Remover test.
type removerTestDeps struct {
	cfg      *Config
	fs       *testutil.MockFileSystem
	ui       *testutil.MockUI
	stateSvc *service.StateManager
}

// newRemoverForTest creates a minimal Remover backed by mocks.
func newRemoverForTest(t *testing.T) (*Remover, *removerTestDeps) {
	t.Helper()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	runner := testutil.RunnerReturning(nil, nil)
	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	st := store.NewStore[service.State](fs, cfg.StatePath)
	stateSvc := service.NewStateManager(st)

	rm := NewRemover(cfg, runner, fs, ui, locker, sys, reg, instReg, stateSvc)
	return rm, &removerTestDeps{cfg: cfg, fs: fs, ui: ui, stateSvc: stateSvc}
}

func TestRemoverRemove_success(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	deps.fs.Files[deps.cfg.RootDir] = []byte{}

	if err := rm.remove(ctx); err != nil {
		t.Fatalf("remove() = %v", err)
	}

	// RootDir should be deleted by RemoveAll.
	if _, err := deps.fs.ReadFile(deps.cfg.RootDir); err == nil {
		t.Error("expected root dir to be removed")
	}
}

func TestRemoverRemove_cancelPrompt(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)
	deps.ui.PromptFunc = func(_ string, _ ...any) bool { return false }
	deps.fs.Files[deps.cfg.RootDir] = []byte{}

	if err := rm.remove(ctx); err != nil {
		t.Fatalf("remove() = %v", err)
	}

	if _, err := deps.fs.ReadFile(deps.cfg.RootDir); err != nil {
		t.Error("expected root dir to remain after cancel")
	}
}

func TestRemoverRemove_dangerousRoot(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)
	deps.cfg.RootDir = "/"

	if err := rm.remove(ctx); err == nil {
		t.Fatal("expected error for dangerous root path")
	}
}

func TestRemoverRemove_removeAllError(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)
	deps.fs.RemoveAllFunc = func(_ string) error { return errMock }

	if err := rm.remove(ctx); err == nil {
		t.Fatal("expected error from RemoveAll")
	}
}

func TestRemoverRemove_linkRemoveWarns(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	deps.fs.Files[deps.cfg.RootDir] = []byte{}
	// Make link removal fail only
	deps.fs.RemoveAllFunc = func(path string) error {
		if path == deps.cfg.LinkPath {
			return errMock
		}
		delete(deps.fs.Files, path)
		return nil
	}

	warnCalled := false
	deps.ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }

	if err := rm.remove(ctx); err != nil {
		t.Fatalf("remove() = %v, want nil despite link error", err)
	}
	if !warnCalled {
		t.Error("expected Warn to be called for link removal failure")
	}
}

func TestRemoverRemove_linkRemoveNoError(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	deps.fs.Files[deps.cfg.RootDir] = []byte{}
	deps.fs.Files[deps.cfg.LinkPath] = []byte{}

	if err := rm.remove(ctx); err != nil {
		t.Fatalf("remove() = %v", err)
	}

	if _, err := deps.fs.ReadFile(filepath.Join(deps.cfg.RootDir, "var", "lock")); err == nil {
	}
}

// Remove (public) — wraps remove() with withRootAndLock.
func TestRemoverRemove_publicMethod(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)
	deps.fs.Files[deps.cfg.RootDir] = []byte{}

	if err := rm.Remove(ctx); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	if _, err := deps.fs.ReadFile(deps.cfg.RootDir); err == nil {
		t.Error("expected root dir to be removed")
	}
}

func TestRemoverRemove_publicMethod_notRoot(t *testing.T) {
	ctx := context.Background()
	_, deps := newRemoverForTest(t)
	deps.fs.Files[deps.cfg.RootDir] = []byte{}
	// Replace the sys on the remoter with a non-root one.
	// We need to recreate since sys is set in NewRemover.
	cfg := deps.cfg
	rm2 := NewRemover(cfg, testutil.RunnerReturning(nil, nil), deps.fs, deps.ui, &testutil.MockLocker{}, &mockSystem{privileged: false}, pkg.NewRegistry(), installer.NewRegistry(), deps.stateSvc)
	if err := rm2.Remove(ctx); err == nil {
		t.Fatal("expected error when not root")
	}
}

// removeManagedPackages tests.

func TestRemoveManagedPackages_stateWithPackage(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	// Write a state file with one package.
	stateJSON := `{"packages": {"test-pkg": {"type": "apt", "variant": "", "version": "1.0"}}}`
	deps.fs.Files[deps.cfg.StatePath] = []byte(stateJSON)

	var warnCalled bool
	deps.ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }

	rm.removeManagedPackages(ctx, &testutil.MockSpinner{})

	// RemoveOne should fail (no package in registry) but the function
	// handles it gracefully by logging a warning.
	if !warnCalled {
		t.Error("expected Warn to be called from RemoveOne failure")
	}
}

func TestRemoveManagedPackages_emptyState(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	// Write an empty state.
	stateJSON := `{"packages": {}}`
	deps.fs.Files[deps.cfg.StatePath] = []byte(stateJSON)

	var warnCalled bool
	deps.ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }

	rm.removeManagedPackages(ctx, &testutil.MockSpinner{})
	if warnCalled {
		t.Error("expected no Warn for empty state")
	}
}

func TestRemoveManagedPackages_stateLoadError(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	// No state file in mock fs → store returns ErrNotFound → Load returns
	// empty state, not an error.
	// To trigger a real load error, write invalid JSON.
	deps.fs.Files[deps.cfg.StatePath] = []byte(`{{{invalid`)

	var warnCalled bool
	deps.ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }

	rm.removeManagedPackages(ctx, &testutil.MockSpinner{})
	if !warnCalled {
		t.Error("expected Warn for state load error")
	}
}
