package self

import (
	"context"
	"fmt"
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

func TestSelectPackages_all(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "a" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c"}
	result := rm.selectPackages(names)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %v", result)
	}
}

func TestSelectPackages_skip(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "0" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c"}
	result := rm.selectPackages(names)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSelectPackages_emptyInput(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c"}
	result := rm.selectPackages(names)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSelectPackages_invalidInput(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "xyz" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c"}
	result := rm.selectPackages(names)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSelectPackages_specific(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "1,3" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c"}
	result := rm.selectPackages(names)
	if len(result) != 2 || result[0] != "pkg-a" || result[1] != "pkg-c" {
		t.Errorf("expected [pkg-a pkg-c], got %v", result)
	}
}

func TestSelectPackages_range(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "2-4" },
	}

	names := []string{"pkg-a", "pkg-b", "pkg-c", "pkg-d", "pkg-e"}
	result := rm.selectPackages(names)
	if len(result) != 3 || result[0] != "pkg-b" || result[1] != "pkg-c" || result[2] != "pkg-d" {
		t.Errorf("expected [pkg-b pkg-c pkg-d], got %v", result)
	}
}

func TestSelectPackages_rangeOutOfBounds(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	rm.logger = &testutil.MockUI{
		PromptInputFunc: func(_, _ string, _ ...any) string { return "1-99" },
	}

	names := []string{"pkg-a", "pkg-b"}
	result := rm.selectPackages(names)
	if result != nil {
		t.Errorf("expected nil for out-of-bounds range, got %v", result)
	}
}

func TestSelectPackages_emptyNames(t *testing.T) {
	rm, _ := newRemoverForTest(t)
	result := rm.selectPackages(nil)
	if result != nil {
		t.Errorf("expected nil for empty names, got %v", result)
	}
}

func TestRemoveManagedPackages_withPackages(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	stateJSON := `{"packages": {"test-pkg": {"type": "apt", "variant": "", "version": "1.0"}}}`
	deps.fs.Files[deps.cfg.StatePath] = []byte(stateJSON)

	st, err := deps.stateSvc.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	var warnCalled bool
	deps.ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }

	rm.removeManagedPackages(ctx, []string{"test-pkg"}, st, &testutil.MockSpinner{})

	if !warnCalled {
		t.Error("expected Warn to be called from RemoveOne failure")
	}
}

func TestRemoveManagedPackages_partial(t *testing.T) {
	ctx := context.Background()
	rm, deps := newRemoverForTest(t)

	stateJSON := `{"packages": {"pkg-a": {"type": "apt"}, "pkg-b": {"type": "apt"}}}`
	deps.fs.Files[deps.cfg.StatePath] = []byte(stateJSON)

	st, err := deps.stateSvc.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	var warns []string
	deps.ui.WarnFunc = func(format string, args ...any) {
		warns = append(warns, fmt.Sprintf(format, args...))
	}

	rm.removeManagedPackages(ctx, []string{"pkg-a"}, st, &testutil.MockSpinner{})

	// Only pkg-a was requested; pkg-b should remain in state.
	if _, ok := st.Packages["pkg-b"]; !ok {
		t.Error("expected pkg-b to remain in state")
	}
}
