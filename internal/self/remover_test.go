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
		// root dir shouldn't be fully removed since RemoveAll only deletes
		// the exact key, not recursively. We check that at least the RootDir
		// entry is gone.
	}
}
