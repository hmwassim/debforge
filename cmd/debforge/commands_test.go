package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

// loadYAMLDefinitions tests

func TestLoadYAMLDefinitions(t *testing.T) {
	reg := pkg.NewRegistry()
	fs := testutil.NewMockFileSystem()
	fs.Files["/test-pkg.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	names := []string{"/test-pkg.yaml"}
	err := loadYAMLDefinitions(reg, names, fs)
	if err != nil {
		t.Fatalf("loadYAMLDefinitions: %v", err)
	}
	p, ok := reg.Lookup("test-pkg")
	if !ok {
		t.Fatal("expected test-pkg registered")
	}
	if p.Name != "test-pkg" || p.Type != pkg.TypeApt {
		t.Errorf("package mismatch: Name=%q Type=%q", p.Name, p.Type)
	}
	if names[0] != "test-pkg" {
		t.Errorf("names[0] = %q, want test-pkg", names[0])
	}
}

func TestLoadYAMLDefinitions_error(t *testing.T) {
	reg := pkg.NewRegistry()
	fs := testutil.NewMockFileSystem()
	fs.Files["/bad.yaml"] = []byte(`{{{`)
	err := loadYAMLDefinitions(reg, []string{"/bad.yaml"}, fs)
	if err == nil {
		t.Fatal("expected error for bad YAML")
	}
}

func TestLoadYAMLDefinitions_skipsNonYAML(t *testing.T) {
	reg := pkg.NewRegistry()
	fs := testutil.NewMockFileSystem()
	err := loadYAMLDefinitions(reg, []string{"non-yaml-name"}, fs)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if _, ok := reg.Lookup("non-yaml-name"); ok {
		t.Error("non-yaml name should not be registered")
	}
}

// loadDefs tests

func TestLoadDefs_success(t *testing.T) {
	reg := pkg.NewRegistry()
	fs := testutil.NewMockFileSystem()
	fs.Files["/test-pkg.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	u := &testutil.MockUI{}
	if ok := loadDefs(reg, []string{"/test-pkg.yaml"}, fs, u); !ok {
		t.Fatal("expected true")
	}
}

func TestLoadDefs_error(t *testing.T) {
	reg := pkg.NewRegistry()
	fs := testutil.NewMockFileSystem()
	fs.Files["/bad.yaml"] = []byte(`{{{`)
	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}
	if ok := loadDefs(reg, []string{"/bad.yaml"}, fs, u); ok {
		t.Fatal("expected false")
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

// withConfirm tests

func TestWithConfirm_cancelled(t *testing.T) {
	var infoCalled string
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return false },
		InfoFunc:   func(format string, _ ...any) { infoCalled = format },
	}
	code := withConfirm(context.Background(), u, func(_ ports.Spinner) error {
		return nil
	})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if infoCalled != "Cancelled" {
		t.Errorf("expected Info call with 'Cancelled', got %q", infoCalled)
	}
}

func TestWithConfirm_success(t *testing.T) {
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
	}
	code := withConfirm(context.Background(), u, func(_ ports.Spinner) error {
		return nil
	})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}

func TestWithConfirm_error(t *testing.T) {
	var errorCalled string
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(format string, _ ...any) { errorCalled = format },
	}
	code := withConfirm(context.Background(), u, func(_ ports.Spinner) error {
		return errors.New("install failed")
	})
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if errorCalled == "" {
		t.Error("expected u.Error to be called")
	}
}

// ---- formatSearchOutput tests ----------------------------------------------

func TestFormatSearchOutput_withResults(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Description: "Package B", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{"pkg-a": {}}}

	out := formatSearchOutput(reg, st, nil)
	if !strings.Contains(out, "[*]") || !strings.Contains(out, "[-]") {
		t.Error("expected both installed [*] and uninstalled [-] markers")
	}
	if !strings.Contains(out, "pkg-a") || !strings.Contains(out, "pkg-b") {
		t.Error("expected both package names in output")
	}
	if !strings.Contains(out, "Package A") || !strings.Contains(out, "Package B") {
		t.Error("expected both descriptions in output")
	}
}

func TestFormatSearchOutput_filtered(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia-driver", Description: "NVIDIA GPU driver", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, []string{"nvidia"})
	if !strings.Contains(out, "nvidia-driver") {
		t.Error("expected nvidia-driver in filtered output")
	}
	if strings.Contains(out, "firefox") {
		t.Error("expected firefox to be filtered out")
	}
}

func TestFormatSearchOutput_matchDescription(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "gpu-tools", Description: "Utilities for NVIDIA GPUs", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "cpu-tools", Description: "CPU utilities", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, []string{"nvidia"})
	if !strings.Contains(out, "gpu-tools") {
		t.Error("expected gpu-tools (matches description 'NVIDIA')")
	}
	if strings.Contains(out, "cpu-tools") {
		t.Error("expected cpu-tools to be filtered out")
	}
}

func TestFormatSearchOutput_noResults(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, []string{"nonexistent"})
	if out != "" {
		t.Errorf("expected empty output for no matches, got %q", out)
	}
}

func TestFormatSearchOutput_emptyPatterns(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, nil)
	if !strings.Contains(out, "pkg-a") || !strings.Contains(out, "pkg-b") {
		t.Error("expected all packages when no patterns")
	}
}

func TestFormatSearchOutput_emptyRegistry(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, nil)
	if out != "" {
		t.Errorf("expected empty output with no packages, got %q", out)
	}
}

func TestFormatSearchOutput_caseInsensitive(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "MyPkg", Description: "My custom package", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "other", Description: "something else", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, []string{"mypkg"})
	if !strings.Contains(out, "MyPkg") {
		t.Error("expected case-insensitive match by name")
	}

	out2 := formatSearchOutput(reg, st, []string{"CUSTOM"})
	if !strings.Contains(out2, "MyPkg") {
		t.Error("expected case-insensitive match by description")
	}
}

func TestFormatSearchOutput_multiplePatternsJoined(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia-driver", Description: "NVIDIA GPU driver", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	// patterns are joined with space and matched as a single substring.
	out := formatSearchOutput(reg, st, []string{"gpu", "driver"})
	if !strings.Contains(out, "nvidia-driver") {
		t.Error("expected nvidia-driver to match 'gpu driver' in description")
	}
	if strings.Contains(out, "firefox") {
		t.Error("expected firefox to be filtered out")
	}
}

func TestWithConfirm_errNotInstalled(t *testing.T) {
	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}
	code := withConfirm(context.Background(), u, func(_ ports.Spinner) error {
		return service.ErrNotInstalled
	})
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if errorCalled {
		t.Error("expected no u.Error call for ErrNotInstalled")
	}
}

// mockInstaller is a trivial installer.Installer test double.
type mockInstaller struct {
	installErr error
	removeErr  error
}

func (m *mockInstaller) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return m.installErr
}

func (m *mockInstaller) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return m.removeErr
}

// mockCmdRunner is a ports.CommandRunner for tests that need to handle
// multiple commands. Each handler is keyed by command name; unmatched
// calls fall through to the default handler (or return nil,nil,nil).
type mockCmdRunner struct {
	handlers map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error)
	def      func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

func (m *mockCmdRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if h, ok := m.handlers[name]; ok {
		return h(ctx, args...)
	}
	if m.def != nil {
		return m.def(ctx, name, args...)
	}
	return nil, nil, nil
}

func (m *mockCmdRunner) RunWithOptions(ctx context.Context, _ ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	return m.Run(ctx, name, args...)
}

var _ ports.CommandRunner = (*mockCmdRunner)(nil)

// newHandlerForTest constructs a commandHandler with the given dependencies,
// skipping the real filesystem/definition loading that newHandler does.
func newHandlerForTest(
	reg *pkg.Registry,
	instReg *installer.Registry,
	stateSvc *service.StateManager,
	locker ports.Locker,
	cfg *self.Config,
	runner ports.CommandRunner,
	fsys ports.FileSystem,
) *commandHandler {
	return &commandHandler{
		reg: reg, instReg: instReg, stateSvc: stateSvc,
		locker: locker, cfg: cfg, runner: runner, fsys: fsys,
	}
}

func TestInstall_success(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

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

func TestRemove_success(t *testing.T) {
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
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				// ListInstalled returns package names.
				if len(args) >= 3 && args[0] == "-W" && args[1] == "-f" && args[2] == "${Package}\n" {
					return []byte("test-pkg\n"), nil, nil
				}
				// IsInstalled returns "installed".
				return []byte("installed\n"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

	var promptCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { promptCalled = true; return true },
	}

	code := h.remove(context.Background(), u, []string{"test-pkg"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !promptCalled {
		t.Error("expected prompt to be called")
	}
}

func TestRemove_notInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "test-pkg", Type: pkg.TypeApt})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeApt, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

	var errorCalled bool
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		ErrorFunc:  func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.remove(context.Background(), u, []string{"test-pkg"})
	// Removing a package not in state is a no-op (not an error).
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if errorCalled {
		t.Error("expected no u.Error call")
	}
}

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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

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

func TestSearch_nonTerminal(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

	// Pipe stdout so term.IsTerminal returns false.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.search(context.Background(), &testutil.MockUI{}, nil)

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read: %v", err)
	}
	output := buf.String()

	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !strings.Contains(output, "pkg-a") {
		t.Errorf("expected output to contain %q, got %q", "pkg-a", output)
	}
	if !strings.Contains(output, "Package A") {
		t.Errorf("expected output to contain %q, got %q", "Package A", output)
	}
}

func TestSearch_loadError(t *testing.T) {
	reg := pkg.NewRegistry()

	// Use a filesystem where Exists returns an error to force a load failure.
	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) { return false, errors.New("stat failed") }
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.search(context.Background(), u, nil)
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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

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
				// No NVIDIA GPU found.
				return []byte("Intel Corporation Device"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

	var warnCalled bool
	u := &testutil.MockUI{
		WarnFunc:   func(_ string, _ ...any) { warnCalled = true },
	}

	code := h.install(context.Background(), u, []string{"nvidia"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !warnCalled {
		t.Error("expected u.Warn to be called for missing GPU")
	}
}

func TestSearch_noResultsWithPattern(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

	var infoCalled bool
	u := &testutil.MockUI{
		InfoFunc: func(_ string, _ ...any) { infoCalled = true },
	}

	code := h.search(context.Background(), u, []string{"nonexistent"})
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if !infoCalled {
		t.Error("expected u.Info to be called for no matches")
	}
}

func TestSearch_emptyRegistryNoPattern(t *testing.T) {
	reg := pkg.NewRegistry()

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys)

	var infoCalled bool
	u := &testutil.MockUI{
		InfoFunc: func(_ string, _ ...any) { infoCalled = true },
	}

	code := h.search(context.Background(), u, nil)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
	if infoCalled {
		t.Error("expected no u.Info call for empty registry without patterns")
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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys)

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

// ---- newHandler tests --------------------------------------------------------

type newHandlerFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (n newHandlerFileInfo) Name() string { return n.name }
func (n newHandlerFileInfo) Size() int64  { return n.size }
func (n newHandlerFileInfo) IsDir() bool  { return n.isDir }

func TestNewHandler_success(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	pkgsDir := "/pkgs"
	statePath := "/state.json"

	fsys.Files[pkgsDir] = nil
	fsys.Files[pkgsDir+"/test-pkg.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	fsys.Files[statePath] = []byte(`{"packages":{}}`)

	fsys.WalkFunc = func(root string, fn func(string, ports.FileInfo, error) error) error {
		for path := range fsys.Files {
			if root == pkgsDir && strings.HasSuffix(path, ".yaml") {
				if err := fn(path, newHandlerFileInfo{name: "test-pkg.yaml", isDir: false}, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}

	cfg := &self.Config{PkgsDir: pkgsDir, StatePath: statePath, LockPath: "/lock"}
	runner := &mockCmdRunner{}
	h, err := newHandler(cfg, fsys, runner, &testutil.MockLocker{}, &testutil.MockUI{})
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	if h.reg == nil {
		t.Error("expected non-nil reg")
	}
	if _, ok := h.reg.Lookup("test-pkg"); !ok {
		t.Error("expected test-pkg to be registered")
	}
	if h.instReg == nil {
		t.Error("expected non-nil instReg")
	}
	if h.stateSvc == nil {
		t.Error("expected non-nil stateSvc")
	}
}

func TestNewHandler_badYAML(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	pkgsDir := "/pkgs"
	statePath := "/state.json"

	fsys.Files[pkgsDir] = nil
	fsys.Files[pkgsDir+"/bad.yaml"] = []byte(`{{{`)
	fsys.Files[statePath] = []byte(`{"packages":{}}`)

	fsys.WalkFunc = func(root string, fn func(string, ports.FileInfo, error) error) error {
		for path := range fsys.Files {
			if root == pkgsDir && strings.HasSuffix(path, ".yaml") {
				if err := fn(path, newHandlerFileInfo{name: "bad.yaml", isDir: false}, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}

	cfg := &self.Config{PkgsDir: pkgsDir, StatePath: statePath, LockPath: "/lock"}
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{})
	if err == nil {
		t.Fatal("expected error for bad YAML")
	}
	if !strings.Contains(err.Error(), "load definitions") {
		t.Errorf("expected error to contain 'load definitions', got %q", err.Error())
	}
}

func TestNewHandler_stateLoadError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	pkgsDir := "/pkgs"
	statePath := "/state.json"

	fsys.Files[pkgsDir] = nil
	fsys.Files[pkgsDir+"/test-pkg.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	// Invalid JSON for state file.
	fsys.Files[statePath] = []byte(`{{{`)

	fsys.WalkFunc = func(root string, fn func(string, ports.FileInfo, error) error) error {
		for path := range fsys.Files {
			if root == pkgsDir && strings.HasSuffix(path, ".yaml") {
				if err := fn(path, newHandlerFileInfo{name: "test-pkg.yaml", isDir: false}, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}

	cfg := &self.Config{PkgsDir: pkgsDir, StatePath: statePath, LockPath: "/lock"}
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{})
	if err == nil {
		t.Fatal("expected error for bad state")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected error to contain 'load state', got %q", err.Error())
	}
}

func TestNewHandler_missingPkgsDir(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	// No pkgsDir in fsys.Files — Exists returns false, LoadAll is a no-op.
	statePath := "/state.json"
	fsys.Files[statePath] = []byte(`{"packages":{}}`)

	cfg := &self.Config{PkgsDir: "/nonexistent", StatePath: statePath, LockPath: "/lock"}
	h, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{})
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	if h.reg == nil {
		t.Error("expected non-nil reg")
	}
}

func TestNewHandler_loadStateExistsError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	pkgsDir := "/pkgs"
	statePath := "/state.json"

	fsys.Files[pkgsDir] = nil
	fsys.Files[pkgsDir+"/test-pkg.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	// Exists returns an error for the state path, which propagates through Load.
	fsys.ExistsFunc = func(path string) (bool, error) {
		if path == statePath {
			return false, errors.New("stat failed")
		}
		_, ok := fsys.Files[path]
		return ok, nil
	}

	fsys.WalkFunc = func(root string, fn func(string, ports.FileInfo, error) error) error {
		for path := range fsys.Files {
			if root == pkgsDir && strings.HasSuffix(path, ".yaml") {
				if err := fn(path, newHandlerFileInfo{name: "test-pkg.yaml", isDir: false}, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}

	cfg := &self.Config{PkgsDir: pkgsDir, StatePath: statePath, LockPath: "/lock"}
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{})
	if err == nil {
		t.Fatal("expected error for state stat failure")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected error to contain 'load state', got %q", err.Error())
	}
}
