package main

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
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

// ---- expandGlobs tests ----------------------------------------------------

func TestExpandGlobs_noGlob(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "firefox"})
	reg.Register(&pkg.Package{Name: "vim"})
	result := expandGlobs(reg, []string{"firefox", "vim"})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestExpandGlobs_globExpands(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "fonts-nerd-fira"})
	reg.Register(&pkg.Package{Name: "fonts-nerd-hack"})
	reg.Register(&pkg.Package{Name: "other-pkg"})
	result := expandGlobs(reg, []string{"fonts-nerd-*"})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d: %v", len(result), result)
	}
}

func TestExpandGlobs_shortPrefixTreatedAsLiteral(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "f*"})
	result := expandGlobs(reg, []string{"f*"})
	if len(result) != 1 || result[0] != "f*" {
		t.Errorf("expected literal 'f*', got %v", result)
	}
}

func TestExpandGlobs_dedup(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "fonts-nerd-hack"})
	reg.Register(&pkg.Package{Name: "fonts-nerd-fira"})
	result := expandGlobs(reg, []string{"fonts-nerd-*", "fonts-nerd-hack"})
	if len(result) != 2 {
		t.Errorf("expected 2 (hack deduped), got %d: %v", len(result), result)
	}
}

func TestExpandGlobs_globNoMatch(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "firefox"})
	result := expandGlobs(reg, []string{"fonts-nerd-*"})
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestExpandGlobs_categoryExpands(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers"})
	result := expandGlobs(reg, []string{"@gaming"})
	if len(result) != 2 {
		t.Errorf("expected 2 gaming packages, got %d: %v", len(result), result)
	}
}

func TestExpandGlobs_categoryNoMatch(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming"})
	result := expandGlobs(reg, []string{"@nonexistent"})
	if len(result) != 1 || result[0] != "@nonexistent" {
		t.Errorf("expected [@nonexistent], got %v", result)
	}
}

func TestExpandGlobs_categoryDedup(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming"})
	result := expandGlobs(reg, []string{"@gaming", "steam"})
	if len(result) != 2 {
		t.Errorf("expected 2 (steam deduped), got %d: %v", len(result), result)
	}
}

func TestExpandGlobs_categoryAndGlob(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming"})
	reg.Register(&pkg.Package{Name: "steamtinkerlaunch", Category: "gaming"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers"})
	result := expandGlobs(reg, []string{"@gaming", "firefox"})
	if len(result) != 3 {
		t.Errorf("expected 3 (2 gaming + 1 literal), got %d: %v", len(result), result)
	}
}

func TestContainsGlob(t *testing.T) {
	if !containsGlob("foo*") {
		t.Error("expected true for *")
	}
	if !containsGlob("foo?") {
		t.Error("expected true for ?")
	}
	if !containsGlob("[abc]") {
		t.Error("expected true for [")
	}
	if containsGlob("literal") {
		t.Error("expected false for literal")
	}
}

func TestGlobPrefixLen(t *testing.T) {
	if n := globPrefixLen("abc*"); n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
	if n := globPrefixLen("ab*"); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if n := globPrefixLen("*"); n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
	if n := globPrefixLen("no-glob"); n != 7 {
		t.Errorf("expected 7, got %d", n)
	}
}
