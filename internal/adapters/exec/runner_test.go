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
