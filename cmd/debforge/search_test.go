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
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
