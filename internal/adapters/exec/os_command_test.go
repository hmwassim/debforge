package exec

import (
	"context"
	"testing"
)

func TestRunEcho(t *testing.T) {
	r := NewOSCommandRunner()
	stdout, stderr, err := r.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "hello\n" && string(stdout) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", string(stdout))
	}
	if len(stderr) != 0 {
		t.Fatalf("expected empty stderr, got '%s'", string(stderr))
	}
}

func TestRunNonExistent(t *testing.T) {
	r := NewOSCommandRunner()
	_, _, err := r.Run(context.Background(), "nonexistent-command-xyz")
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestRunWithArgs(t *testing.T) {
	r := NewOSCommandRunner()
	stdout, _, err := r.Run(context.Background(), "sh", "-c", "echo arg1 arg2")
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "arg1 arg2\n" && string(stdout) != "arg1 arg2" {
		t.Fatalf("expected 'arg1 arg2', got '%s'", string(stdout))
	}
}

func TestRunContextCancelled(t *testing.T) {
	r := NewOSCommandRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
