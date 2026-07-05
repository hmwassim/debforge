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
	if !strings.Contains(output, "gaming") || !strings.Contains(output, "(0/1)") {
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
