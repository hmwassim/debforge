package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestRunScript_surfacesStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("configure: error: missing libfoo\n"), errors.New("exit status 1")
		},
	}

	spinner := &testutil.MockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./configure", "configuring")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing libfoo") {
		t.Errorf("error should contain stderr output, got: %v", err)
	}
}

func TestRunScript_noStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}

	spinner := &testutil.MockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), ": :") {
		t.Errorf("error should not have empty stderr suffix: %v", err)
	}
}

func TestRunScriptInDir_surfacesStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("error: build failed"), errors.New("exit status 1")
		},
	}
	spinner := &testutil.MockSpinner{}
	err := RunScriptInDir(context.Background(), runner, spinner, "test-pkg", "./build.sh", "building", "/tmp/build")
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Errorf("expected error with stderr, got %v", err)
	}
}

func TestRunScriptInDir_success(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("ok"), nil, nil
		},
	}
	spinner := &testutil.MockSpinner{}
	err := RunScriptInDir(context.Background(), runner, spinner, "test-pkg", "./build.sh", "building", "/tmp/build")
	if err != nil {
		t.Errorf("RunScriptInDir: %v", err)
	}
}

func TestRunPostInstall_empty(t *testing.T) {
	err := RunPostInstall(context.Background(), nil, nil, "test", "")
	if err != nil {
		t.Errorf("RunPostInstall empty: %v", err)
	}
}

func TestRunPostRemove_empty(t *testing.T) {
	err := RunPostRemove(context.Background(), nil, nil, "test", "")
	if err != nil {
		t.Errorf("RunPostRemove empty: %v", err)
	}
}

func TestMkdirTemp(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.TempDir = "/tmp/debforge-test123"
	dir, err := MkdirTemp(fs)
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	if dir != "/tmp/debforge-test123" {
		t.Errorf("dir = %q, want %q", dir, "/tmp/debforge-test123")
	}
}

func TestWithTempDir_success(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.TempDir = "/tmp/debforge-test123"
	var gotDir string
	err := WithTempDir(fs, "test", func(dir string) error {
		gotDir = dir
		return nil
	})
	if err != nil {
		t.Fatalf("WithTempDir: %v", err)
	}
	if gotDir != "/tmp/debforge-test123" {
		t.Errorf("dir = %q, want %q", gotDir, "/tmp/debforge-test123")
	}
}

func TestWithTempDir_fnError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.TempDir = "/tmp/debforge-test123"
	err := WithTempDir(fs, "test", func(_ string) error {
		return errors.New("fn failed")
	})
	if err == nil || !strings.Contains(err.Error(), "fn failed") {
		t.Errorf("expected 'fn failed' error, got %v", err)
	}
}

func TestWithTempDir_cleanupError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.TempDir = "/tmp/debforge-test123"
	fs.RemoveAllFunc = func(_ string) error {
		return errors.New("cleanup failed")
	}
	err := WithTempDir(fs, "test", func(_ string) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "clean up temp dir") {
		t.Errorf("expected cleanup error, got %v", err)
	}
}

func TestWithTempDir_fnErrorWithCleanupError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.TempDir = "/tmp/debforge-test123"
	fs.RemoveAllFunc = func(_ string) error {
		return errors.New("cleanup failed")
	}
	err := WithTempDir(fs, "test", func(_ string) error {
		return errors.New("fn failed")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fn failed") {
		t.Errorf("expected 'fn failed', got %v", err)
	}
	if !strings.Contains(err.Error(), "cleanup failed") {
		t.Errorf("expected cleanup error to be wrapped, got %v", err)
	}
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRunScript_truncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", 1000)
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte(long), errors.New("exit status 1")
		},
	}

	spinner := &testutil.MockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("long stderr should be truncated ending with ..., got: %v", msg)
	}
	if len(msg) > 600 {
		t.Errorf("error message too long (%d chars), should be truncated", len(msg))
	}
}

var _ ports.Spinner = (*testutil.MockSpinner)(nil)
