//go:build integration

package exec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

func TestRun_echo(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	stdout, stderr, err := r.Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if strings.TrimSpace(string(stdout)) != "hello" {
		t.Errorf("stdout = %q, want %q", string(stdout), "hello\n")
	}
	if len(stderr) != 0 {
		t.Errorf("stderr = %q, want empty", string(stderr))
	}
}

func TestRun_commandNotFound(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	_, _, err := r.Run(ctx, "nonexistent-command-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestRunWithOptions_Dir(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	dir := t.TempDir()

	stdout, _, err := r.RunWithOptions(ctx, ports.RunOptions{Dir: dir}, "pwd")
	if err != nil {
		t.Fatalf("RunWithOptions() = %v", err)
	}
	got := strings.TrimSpace(string(stdout))
	if got != dir {
		t.Errorf("pwd = %q, want %q", got, dir)
	}
}

func TestRunWithOptions_Env(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()

	stdout, _, err := r.RunWithOptions(ctx, ports.RunOptions{
		Env: []string{"TEST_VAR=hello"},
	}, "sh", "-c", "echo $TEST_VAR")
	if err != nil {
		t.Fatalf("RunWithOptions() = %v", err)
	}
	if strings.TrimSpace(string(stdout)) != "hello" {
		t.Errorf("stdout = %q, want %q", string(stdout), "hello")
	}
}

func TestRunWithOptions_Stdout(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	var buf bytes.Buffer

	_, _, err := r.RunWithOptions(ctx, ports.RunOptions{Stdout: &buf}, "echo", "streamed")
	if err != nil {
		t.Fatalf("RunWithOptions() = %v", err)
	}
	if strings.TrimSpace(buf.String()) != "streamed" {
		t.Errorf("stdout = %q, want %q", buf.String(), "streamed\n")
	}
}

func TestRunWithOptions_Stderr(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	var buf bytes.Buffer

	_, _, err := r.RunWithOptions(ctx, ports.RunOptions{Stderr: &buf}, "sh", "-c", "echo errmsg >&2")
	if err != nil {
		t.Fatalf("RunWithOptions() = %v", err)
	}
	if strings.TrimSpace(buf.String()) != "errmsg" {
		t.Errorf("stderr = %q, want %q", buf.String(), "errmsg\n")
	}
}

func TestRun_cancelContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := NewRunner()
	_, _, err := r.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRunWithOptions_extraEnvPreservesInherited(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()

	original := os.Getenv("PATH")
	stdout, _, err := r.RunWithOptions(ctx, ports.RunOptions{
		Env: []string{"EXTRA_VAR=present"},
	}, "sh", "-c", "echo $PATH")
	if err != nil {
		t.Fatalf("RunWithOptions() = %v", err)
	}
	got := strings.TrimSpace(string(stdout))
	// PATH should still be set because Env is appended to os.Environ()
	if got == "" {
		t.Error("expected PATH to be inherited")
	}
	if !strings.Contains(got, string(filepath.ListSeparator)) {
		t.Errorf("PATH = %q, looks truncated", got)
	}
	_ = original
}

func TestSetLogFn_calledOnSuccess(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	var called bool
	var gotName string
	r.SetLogFn(func(name string, args []string, stdout, stderr []byte, err error) {
		called = true
		gotName = name
	})
	_, _, err := r.Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if !called {
		t.Error("logFn should have been called")
	}
	if gotName != "echo" {
		t.Errorf("logFn name = %q, want %q", gotName, "echo")
	}
}

func TestSetLogFn_calledOnError(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	var called bool
	var gotErr error
	r.SetLogFn(func(name string, args []string, stdout, stderr []byte, err error) {
		called = true
		gotErr = err
	})
	_, _, _ = r.Run(ctx, "nonexistent-command-12345")
	if !called {
		t.Error("logFn should have been called even on error")
	}
	if gotErr == nil {
		t.Error("logFn should receive the error")
	}
}

func TestSetLogFn_receivesStderr(t *testing.T) {
	ctx := context.Background()
	r := NewRunner()
	var gotStderr []byte
	r.SetLogFn(func(name string, args []string, stdout, stderr []byte, err error) {
		gotStderr = stderr
	})
	_, _, err := r.Run(ctx, "sh", "-c", "echo errmsg >&2")
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if !strings.Contains(string(gotStderr), "errmsg") {
		t.Errorf("logFn stderr = %q, want it to contain %q", string(gotStderr), "errmsg")
	}
}
