package dpkg

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestCachedRunner_caches(t *testing.T) {
	var mu sync.Mutex
	var callCount int
	inner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			// ListInstalled-style query
			return []byte("bash\ndpkg\napt\n"), nil, nil
		},
	}

	cr := NewCachedRunner(inner)

	// First call populates cache
	stdout, _, err := cr.Run(context.Background(), "dpkg-query", "-W", "-f=${db:Status-Status}\n", "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "installed\n" {
		t.Errorf("first call: got %q, want %q", string(stdout), "installed\n")
	}

	// Second call uses cache
	stdout, _, err = cr.Run(context.Background(), "dpkg-query", "-W", "-f=${db:Status-Status}\n", "dpkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "installed\n" {
		t.Errorf("second call: got %q, want %q", string(stdout), "installed\n")
	}

	// Unknown package from cache
	stdout, _, err = cr.Run(context.Background(), "dpkg-query", "-W", "-f=${db:Status-Status}\n", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "not-installed\n" {
		t.Errorf("missing pkg: got %q, want %q", string(stdout), "not-installed\n")
	}

	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected 1 call to inner runner, got %d", callCount)
	}
	mu.Unlock()
}

func TestCachedRunner_passthrough(t *testing.T) {
	inner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" && strings.Join(args, " ") == "-W -f ${Package}\n" {
				return []byte("passthrough-output"), nil, nil
			}
			return nil, nil, errors.New("unexpected call")
		},
	}

	cr := NewCachedRunner(inner)

	// ListInstalled-style query should pass through
	stdout, stderr, err := cr.Run(context.Background(), "dpkg-query", "-W", "-f", "${Package}\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stderr) > 0 {
		t.Errorf("unexpected stderr: %s", string(stderr))
	}
	if string(stdout) != "passthrough-output" {
		t.Errorf("got %q, want %q", string(stdout), "passthrough-output")
	}
}

func TestCachedRunner_contextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inner := &testutil.MockRunner{
		RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.Canceled
		},
	}

	cr := NewCachedRunner(inner)
	_, _, err := cr.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}\n", "bash")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestCachedRunner_nonDpkgQueryPassthrough(t *testing.T) {
	inner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("other-output"), nil, nil
		},
	}

	cr := NewCachedRunner(inner)

	// A command that is not dpkg-query should pass through without caching
	stdout, _, err := cr.Run(context.Background(), "apt-get", "update")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "other-output" {
		t.Errorf("got %q, want %q", string(stdout), "other-output")
	}
}
