package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

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
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

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

func TestRemove_warnsOnCascade(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "scx-scheds",
		Type: pkg.TypeDeb,
		Deb:  &pkg.DebConfig{Package: "scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-tools",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-tools"},
		Depends: []string{"scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-switcher",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-switcher"},
		Depends: []string{"scx-scheds", "scx-tools"},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeDeb, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"scx-scheds":{"type":"deb"},"scx-tools":{"type":"deb"},"scx-switcher":{"type":"deb"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				if len(args) >= 3 && args[0] == "-W" && args[1] == "-f" && args[2] == "${Package}\n" {
					return []byte("scx-scheds\nscx-tools\nscx-switcher\n"), nil, nil
				}
				return []byte("installed\n"), nil, nil
			},
		},
	}
	h := newHandlerForTest(reg, instReg, stateSvc, &testutil.MockLocker{}, cfg, runner, fsys, &testutil.MockSystem{})

	var infoMsg string
	u := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return true },
		InfoFunc:   func(format string, args ...any) { infoMsg = fmt.Sprintf(format, args...) },
	}

	code := h.remove(context.Background(), u, []string{"scx-scheds"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if infoMsg == "" {
		t.Fatal("expected Info call with dependent names")
	}
	if !strings.Contains(infoMsg, "scx-tools") || !strings.Contains(infoMsg, "scx-switcher") {
		t.Errorf("expected Info to mention dependents, got %q", infoMsg)
	}
}

func TestRemove_noWarnWithoutCascade(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "scx-scheds",
		Type: pkg.TypeDeb,
		Deb:  &pkg.DebConfig{Package: "scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-tools",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-tools"},
		Depends: []string{"scx-scheds"},
	})
	reg.Register(&pkg.Package{
		Name:    "scx-switcher",
		Type:    pkg.TypeDeb,
		Deb:     &pkg.DebConfig{Package: "scx-switcher"},
		Depends: []string{"scx-scheds", "scx-tools"},
	})

	instReg := installer.NewRegistry()
	instReg.Register(pkg.TypeDeb, &mockInstaller{})

	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte(`{"packages":{"scx-scheds":{"type":"deb"},"scx-tools":{"type":"deb"},"scx-switcher":{"type":"deb"}}}`)
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	runner := &mockCmdRunner{
		handlers: map[string]func(ctx context.Context, args ...string) ([]byte, []byte, error){
			"dpkg-query": func(_ context.Context, args ...string) ([]byte, []byte, error) {
				if len(args) >= 3 && args[0] == "-W" && args[1] == "-f" && args[2] == "${Package}\n" {
					return []byte("scx-scheds\nscx-tools\nscx-switcher\n"), nil, nil
				}
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

	code := h.remove(context.Background(), u, []string{"scx-switcher"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if infoCalled {
		t.Error("expected no Info call when no cascading dependents")
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

func TestSearch_nonTerminal(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

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
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

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

func TestSearch_noResultsWithPattern(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

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
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

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

// ---- selectPager tests ------------------------------------------------------

func TestSelectPager_pagerEnvNoArgs(t *testing.T) {
	t.Setenv("PAGER", "mypager")
	oldLookPath := lookPath
	lookPath = func(_ string) (string, error) { return "", nil }
	t.Cleanup(func() { lookPath = oldLookPath })

	cmd, args := selectPager()
	if cmd != "mypager" {
		t.Errorf("expected cmd mypager, got %q", cmd)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

func TestSelectPager_pagerEnvWithArgs(t *testing.T) {
	t.Setenv("PAGER", "mypager -F -X")
	oldLookPath := lookPath
	lookPath = func(_ string) (string, error) { return "", nil }
	t.Cleanup(func() { lookPath = oldLookPath })

	cmd, args := selectPager()
	if cmd != "mypager" {
		t.Errorf("expected cmd mypager, got %q", cmd)
	}
	want := []string{"-F", "-X"}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Errorf("expected args %v, got %v", want, args)
	}
}

func TestSelectPager_lessFound(t *testing.T) {
	oldLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name == "less" {
			return "/usr/bin/less", nil
		}
		return "", nil
	}
	t.Cleanup(func() { lookPath = oldLookPath })

	cmd, args := selectPager()
	if cmd != "/usr/bin/less" {
		t.Errorf("expected cmd /usr/bin/less, got %q", cmd)
	}
	if len(args) != 1 || args[0] != "-FRSX" {
		t.Errorf("expected args [-FRSX], got %v", args)
	}
}

func TestSelectPager_noPagerAvailable(t *testing.T) {
	oldLookPath := lookPath
	lookPath = func(_ string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = oldLookPath })

	cmd, args := selectPager()
	if cmd != "" {
		t.Errorf("expected empty cmd, got %q", cmd)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

// ---- search pager (integration) tests ----------------------------------------

func TestSearch_pagerSuccess(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	// Force isTerminal=true and set PAGER to cat.
	oldTerm := isTerminal
	isTerminal = func(_ int) bool { return true }
	t.Cleanup(func() { isTerminal = oldTerm })
	t.Setenv("PAGER", "cat")

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
}

func TestSearch_pagerFails(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	oldTerm := isTerminal
	isTerminal = func(_ int) bool { return true }
	t.Cleanup(func() { isTerminal = oldTerm })
	// false exits with code 1, triggering fallback to fmt.Print.
	t.Setenv("PAGER", "false")

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
		t.Errorf("expected fallback output to contain %q, got %q", "pkg-a", output)
	}
}

func TestSearch_noPagerAvailableTerminal(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	oldTerm := isTerminal
	isTerminal = func(_ int) bool { return true }
	t.Cleanup(func() { isTerminal = oldTerm })
	oldLook := lookPath
	lookPath = func(_ string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = oldLook })

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
		t.Errorf("expected fallback output to contain %q, got %q", "pkg-a", output)
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

// ---- newHandler tests --------------------------------------------------------

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
	h, err := newHandler(cfg, fsys, runner, &testutil.MockLocker{}, &testutil.MockUI{}, &testutil.MockSystem{})
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
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{}, &testutil.MockSystem{})
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
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{}, &testutil.MockSystem{})
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
	h, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{}, &testutil.MockSystem{})
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
	_, err := newHandler(cfg, fsys, &mockCmdRunner{}, &testutil.MockLocker{}, &testutil.MockUI{}, &testutil.MockSystem{})
	if err == nil {
		t.Fatal("expected error for state stat failure")
	}
	if !strings.Contains(err.Error(), "load state") {
		t.Errorf("expected error to contain 'load state', got %q", err.Error())
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

// ---- list handler -----------------------------------------------------------

func TestList_nonTerminal(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.list(context.Background(), &testutil.MockUI{}, "", false)

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
	if !strings.Contains(output, "gaming") || !strings.Contains(output, "(1)") {
		t.Errorf("expected categories in output, got %q", output)
	}
}

func TestList_category(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.list(context.Background(), &testutil.MockUI{}, "gaming", false)

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
	if !strings.Contains(output, "steam") {
		t.Errorf("expected steam in output, got %q", output)
	}
}

func TestList_loadError(t *testing.T) {
	reg := pkg.NewRegistry()

	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) { return false, errors.New("stat failed") }
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.list(context.Background(), u, "", false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

// ---- info handler (non-terminal) --------------------------------------------

func TestInfo_nonTerminal(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt, Category: "browsers", Packages: []string{"firefox"}})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.info(context.Background(), &testutil.MockUI{}, []string{"firefox"}, false)

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
	if !strings.Contains(output, "firefox") {
		t.Errorf("expected firefox in output, got %q", output)
	}
}

func TestInfo_unknownPackage(t *testing.T) {
	reg := pkg.NewRegistry()

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.info(context.Background(), u, []string{"nonexistent"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

func TestInfo_loadError(t *testing.T) {
	reg := pkg.NewRegistry()

	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) { return false, errors.New("stat failed") }
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	var errorCalled bool
	u := &testutil.MockUI{
		ErrorFunc: func(_ string, _ ...any) { errorCalled = true },
	}

	code := h.info(context.Background(), u, []string{"pkg"}, false)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
	if !errorCalled {
		t.Error("expected u.Error to be called")
	}
}

func TestInfo_multiplePackages(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Category: "cat1", Packages: []string{"a"}})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt, Category: "cat2", Packages: []string{"b"}})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.info(context.Background(), &testutil.MockUI{}, []string{"pkg-a", "pkg-b"}, false)

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
	if !strings.Contains(output, "pkg-a") || !strings.Contains(output, "pkg-b") {
		t.Errorf("expected both packages in output, got %q", output)
	}
}

func TestInfo_pagerSuccess(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt, Category: "cat", Packages: []string{"a"}})

	fsys := testutil.NewMockFileSystem()
	st := store.NewStore[service.State](fsys, "/state.json")
	stateSvc := service.NewStateManager(st)

	cfg := &self.Config{LockPath: "/lock"}
	h := newHandlerForTest(reg, installer.NewRegistry(), stateSvc, &testutil.MockLocker{}, cfg, testutil.RunnerReturning(nil, nil), fsys, &testutil.MockSystem{})

	oldTerm := isTerminal
	isTerminal = func(_ int) bool { return true }
	t.Cleanup(func() { isTerminal = oldTerm })
	t.Setenv("PAGER", "cat")

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	code := h.info(context.Background(), &testutil.MockUI{}, []string{"pkg-a"}, false)

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
		t.Errorf("expected pkg-a in output, got %q", output)
	}
}
