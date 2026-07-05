package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
