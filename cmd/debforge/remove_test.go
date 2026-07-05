package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
