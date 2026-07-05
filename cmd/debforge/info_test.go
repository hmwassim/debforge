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
