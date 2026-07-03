package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestUsage(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	usage()

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read: %v", err)
	}
	output := buf.String()

	if len(output) == 0 {
		t.Error("usage() output is empty")
	}
	if !strings.Contains(output, "debforge") {
		t.Errorf("usage() output does not contain %q: %q", "debforge", output)
	}
}

func newRunWithEnv(t *testing.T) (*testutil.MockFileSystem, *mockCmdRunner, *testutil.MockLocker, *testutil.MockSystem, *testutil.MockUI, *self.Config) {
	t.Helper()
	fsys := testutil.NewMockFileSystem()
	cfg := &self.Config{PkgsDir: "/pkgs", LockPath: "/lock", StatePath: "/state.json"}
	fsys.Files["/state.json"] = []byte(`{"packages":{}}`)
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"apt-get": func(_ context.Context, _ ...string) ([]byte, []byte, error) {
				return nil, nil, nil
			},
		},
	}
	locker := &testutil.MockLocker{}
	sys := &testutil.MockSystem{Privileged: true}
	ui := &testutil.MockUI{}
	return fsys, runner, locker, sys, ui, cfg
}

func runWithArgs(ctx context.Context, args []string, fsys ports.FileSystem, runner ports.CommandRunner, locker ports.Locker, sys ports.System, ui ports.UI, cfg *self.Config) int {
	return runWith(ctx, args, "testver", cfg, fsys, runner, locker, sys, ui)
}

// ---- no args / special commands --------------------------------------------

func TestRunWith_noArgs(t *testing.T) {
	_, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), nil, nil, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_version(t *testing.T) {
	_, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"--version"}, nil, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_help(t *testing.T) {
	_, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"--help"}, nil, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_unknownCommand(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"foobar"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

// ---- --self-update / --self-remove (not privileged) -------------------------

func TestRunWith_selfUpdate_notPrivileged(t *testing.T) {
	fsys, runner, locker, _, ui, cfg := newRunWithEnv(t)
	sys := &testutil.MockSystem{}
	code := runWithArgs(context.Background(), []string{"--self-update"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_selfRemove_notPrivileged(t *testing.T) {
	fsys, runner, locker, _, ui, cfg := newRunWithEnv(t)
	sys := &testutil.MockSystem{}
	code := runWithArgs(context.Background(), []string{"--self-remove"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

// ---- install ----------------------------------------------------------------

func TestRunWith_install_noNames(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"install"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_install_unknownPackage(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	var errorCalled bool
	ui.ErrorFunc = func(_ string, _ ...any) { errorCalled = true }
	code := runWithArgs(context.Background(), []string{"install", "nonexistent"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected ui.Error to be called")
	}
}

func TestRunWith_install_withYesFlag(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	var errorCalled bool
	ui.ErrorFunc = func(_ string, _ ...any) { errorCalled = true }
	code := runWithArgs(context.Background(), []string{"-y", "install", "nonexistent"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !ui.Yes {
		t.Error("expected Yes mode to be set")
	}
	if !errorCalled {
		t.Error("expected ui.Error to be called")
	}
}

// ---- remove -----------------------------------------------------------------

func TestRunWith_remove_noNames(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"remove"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_remove_unknownPackage(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	var errorCalled bool
	ui.ErrorFunc = func(_ string, _ ...any) { errorCalled = true }
	code := runWithArgs(context.Background(), []string{"remove", "nonexistent"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected ui.Error to be called")
	}
}

// ---- update ----------------------------------------------------------------

func TestRunWith_update_noNamesNoAll(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"update"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_update_unknownPackage(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
	var errorCalled bool
	ui.ErrorFunc = func(_ string, _ ...any) { errorCalled = true }
	code := runWithArgs(context.Background(), []string{"update", "nonexistent"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected ui.Error to be called")
	}
}

func TestRunWith_update_allMode(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
	code := runWithArgs(context.Background(), []string{"update", "--all"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_update_allModeWithName(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
	var warnCalled bool
	ui.WarnFunc = func(_ string, _ ...any) { warnCalled = true }
	code := runWithArgs(context.Background(), []string{"update", "--all", "somepkg"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !warnCalled {
		t.Error("expected ui.Warn to be called")
	}
}

// ---- search ----------------------------------------------------------------

func TestRunWith_search_empty(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"search"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_searchWithPattern(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	var infoCalled bool
	ui.InfoFunc = func(_ string, _ ...any) { infoCalled = true }
	code := runWithArgs(context.Background(), []string{"search", "pattern"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !infoCalled {
		t.Error("expected ui.Info to be called")
	}
}

// ---- flag parsing ----------------------------------------------------------

func TestRunWith_flagParseError(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	var errorCalled bool
	ui.ErrorFunc = func(_ string, _ ...any) { errorCalled = true }
	code := runWithArgs(context.Background(), []string{"--unknown-flag"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected ui.Error to be called")
	}
}

// ---- flags only (no command) -----------------------------------------------

func TestRunWith_flagsOnly(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"-y"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestRunWith_flagHelp(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"-h", "install"}, fsys, runner, locker, sys, ui, cfg)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

// ---- loadDefs errors -------------------------------------------------------

func TestRunWith_install_loadDefsError(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"install", "nonexistent.yaml"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_remove_loadDefsError(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	code := runWithArgs(context.Background(), []string{"remove", "nonexistent.yaml"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestRunWith_update_loadDefsError(t *testing.T) {
	fsys, runner, locker, sys, ui, cfg := newRunWithEnv(t)
	originalAptExec := aptpty.AptExec
	t.Cleanup(func() { aptpty.AptExec = originalAptExec })
	aptpty.AptExec = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
	code := runWithArgs(context.Background(), []string{"update", "nonexistent.yaml"}, fsys, runner, locker, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

// ---- bootstrap error -------------------------------------------------------

func TestRunWith_selfRemove_bootstrapError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{{{invalid json}}}`)
	cfg := &self.Config{PkgsDir: "/pkgs", LockPath: "/lock", StatePath: "/state.json"}
	runner := &mockCmdRunner{}
	sys := &testutil.MockSystem{Privileged: true}
	ui := &testutil.MockUI{}
	var errorCalled string
	ui.ErrorFunc = func(fmt string, args ...any) {
		errorCalled = fmt
	}
	code := runWithArgs(context.Background(), []string{"--self-remove"}, fsys, runner, &testutil.MockLocker{}, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !strings.Contains(errorCalled, "bootstrap") {
		t.Error("expected bootstrap error")
	}
}

func TestRunWith_bootstrapError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{{{invalid json}}}`)
	cfg := &self.Config{PkgsDir: "/pkgs", LockPath: "/lock", StatePath: "/state.json"}
	runner := &mockCmdRunner{}
	sys := &testutil.MockSystem{Privileged: true}
	ui := &testutil.MockUI{}
	var errorCalled string
	ui.ErrorFunc = func(fmt string, args ...any) {
		errorCalled = fmt
	}
	code := runWithArgs(context.Background(), []string{"install", "pkg"}, fsys, runner, &testutil.MockLocker{}, sys, ui, cfg)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !strings.Contains(errorCalled, "bootstrap") {
		t.Error("expected bootstrap error")
	}
}
