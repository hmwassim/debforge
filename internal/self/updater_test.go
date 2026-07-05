package self

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

var errMock = errors.New("mock error")

type mockSystem struct {
	privileged bool
}

func (m *mockSystem) IsPrivileged() bool           { return m.privileged }
func (m *mockSystem) Getenv(_ string) string       { return "" }
func (m *mockSystem) UserHomeDir() (string, error) { return "/home/test", nil }
func (m *mockSystem) LookupUser(_ string) (*ports.UserInfo, error) {
	return &ports.UserInfo{HomeDir: "/home/test", Uid: 1000, Gid: 1000}, nil
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func TestUpdaterUpdate_freshInstall(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case name == "git" && len(args) >= 1 && args[0] == "clone":
				return nil, nil, nil
			case name == "git" && len(args) >= 3 && args[0] == "-C" && args[2] == "describe":
				return []byte("v1.0.0\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			case strings.HasSuffix(name, "debforge.new") && len(args) == 1 && args[0] == "--version":
				return []byte("v1.0.0\n"), nil, nil
			}
			return nil, nil, fmt.Errorf("unexpected: %s %v", name, args)
		},
	}
	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err != nil {
		t.Fatalf("update() = %v", err)
	}

	finalPath := filepath.Join(cfg.BinDir, "debforge")
	if _, ok := fs.Files[finalPath]; !ok {
		t.Errorf("expected final binary at %s", finalPath)
	}
}

func TestUpdaterUpdate_alreadyUpToDate(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	revCount := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "git" && len(args) >= 2 && args[0] == "-C" {
				revCount++
				return []byte("abc123\n"), nil, nil
			}
			return nil, nil, nil
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err != nil {
		t.Fatalf("update() = %v", err)
	}

	if revCount != 3 {
		t.Errorf("expected 3 git -C calls (fetch, HEAD, origin), got %d", revCount)
	}
}

func TestUpdaterUpdate_updateAvailable(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case name == "git" && len(args) >= 3 && args[0] == "-C" && args[2] == "fetch" && !contains(args, "--depth"):
				return []byte(""), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			case name == "git" && len(args) >= 3 && args[0] == "-C" && args[2] == "fetch" && contains(args, "--depth"):
				return []byte(""), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "reset":
				return []byte(""), nil, nil
			case name == "git" && len(args) >= 3 && args[0] == "-C" && args[2] == "describe":
				return []byte("v1.0.0\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			case strings.HasSuffix(name, "debforge.new") && len(args) == 1 && args[0] == "--version":
				return []byte("v1.0.0\n"), nil, nil
			}
			return nil, nil, fmt.Errorf("unexpected: %s %v", name, args)
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err != nil {
		t.Fatalf("update() = %v", err)
	}

	if _, ok := fs.Files[filepath.Join(cfg.BinDir, "debforge")]; !ok {
		t.Error("expected final binary after update")
	}
}

func TestUpdaterUpdate_cancelInstallPrompt(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	runner := testutil.RunnerReturning(nil, nil)
	ui := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return false },
	}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err != nil {
		t.Fatalf("update() = %v, want nil", err)
	}
}

func TestUpdaterUpdate_cancelUpdatePrompt(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	promptCount := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "git" && len(args) >= 2 && args[0] == "-C" {
				promptCount++
				if promptCount <= 2 {
					return []byte("abc123\n"), nil, nil
				}
				return []byte("def456\n"), nil, nil
			}
			return nil, nil, nil
		},
	}

	ui := &testutil.MockUI{
		PromptFunc: func(_ string, _ ...any) bool { return false },
	}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err != nil {
		t.Fatalf("update() = %v, want nil", err)
	}
}

func TestUpdaterUpdate_cloneError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "git" {
				return nil, nil, errMock
			}
			return nil, nil, nil
		},
	}
	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from clone")
	}
}

func TestUpdaterUpdate_buildError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case name == "go" && args[0] == "build":
				return nil, nil, errMock
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			default:
				return []byte("abc123\n"), nil, nil
			}
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from build")
	}
}

func TestUpdaterUpdate_verifyBinaryError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case strings.HasSuffix(name, "debforge.new"):
				return nil, []byte("error"), errMock
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			default:
				return []byte("abc123\n"), nil, nil
			}
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from verify")
	}
}

func TestUpdaterUpdate_verifyNoOutput(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case strings.HasSuffix(name, "debforge.new"):
				return nil, nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			default:
				return []byte("abc123\n"), nil, nil
			}
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from verify (no stdout)")
	}
}

func TestUpdaterUpdate_mkdirAllError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.MkdirAllFunc = func(_ string, _ int) error { return errMock }
	runner := testutil.RunnerReturning(nil, nil)
	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from MkdirAll")
	}
}

func TestUpdaterUpdate_installBinaryError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}
	fs.RenameFunc = func(_, _ string) error { return errMock }

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case strings.HasSuffix(name, "debforge.new"):
				return []byte("v1.0.0\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			default:
				return []byte("abc123\n"), nil, nil
			}
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from installBinary (Rename)")
	}
}

func TestUpdaterUpdate_ensureLinkError(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}
	fs.SymlinkFunc = func(_, _ string) error { return errMock }

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case strings.HasSuffix(name, "debforge.new"):
				return []byte("v1.0.0\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && args[3] == "HEAD":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 4 && args[0] == "-C" && args[2] == "rev-parse" && strings.HasPrefix(args[3], "origin/"):
				return []byte("def456\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			default:
				return []byte("abc123\n"), nil, nil
			}
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.update(ctx); err == nil {
		t.Fatal("expected error from ensureLink (Symlink)")
	}
}

func TestUpdaterUpdate_publicMethod(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files[filepath.Join(cfg.SourceDir, ".git")] = []byte{}

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			switch {
			case name == "git" && len(args) >= 2 && args[0] == "-C":
				return []byte("abc123\n"), nil, nil
			case name == "git" && len(args) >= 3 && args[0] == "describe":
				return []byte("v1.0.0\n"), nil, nil
			case name == "go" && len(args) >= 3 && args[0] == "build" && args[1] == "-o":
				fs.Files[args[2]] = []byte("binary")
				return nil, nil, nil
			case strings.HasSuffix(name, "debforge.new") && len(args) == 1 && args[0] == "--version":
				return []byte("v1.0.0\n"), nil, nil
			}
			return nil, nil, fmt.Errorf("unexpected: %s %v", name, args)
		},
	}

	ui := &testutil.MockUI{Yes: true}
	locker := &testutil.MockLocker{}
	sys := &mockSystem{privileged: true}

	u := NewUpdater(cfg, runner, fs, ui, locker, sys, false)
	if err := u.Update(ctx); err != nil {
		t.Fatalf("Update() = %v", err)
	}
}
