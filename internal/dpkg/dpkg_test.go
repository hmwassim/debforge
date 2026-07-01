package dpkg

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestIsInstalled_installed(t *testing.T) {
	ok, err := IsInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected installed")
	}
}

func TestIsInstalled_notInstalled(t *testing.T) {
	ok, err := IsInstalled(context.Background(), testutil.RunnerReturning([]byte("not-installed\n"), nil), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected not installed")
	}
}

func TestIsInstalled_error(t *testing.T) {
	ok, err := IsInstalled(context.Background(), testutil.RunnerReturning(nil, errors.New("dpkg-query failed")), "broken")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected not installed on error")
	}
}

func TestIsInstalled_cancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := IsInstalled(ctx, testutil.RunnerReturning(nil, context.Canceled), "pkg")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestListInstalled_multiple(t *testing.T) {
	runner := testutil.RunnerReturning([]byte("bash\ndpkg\napt\n"), nil)
	installed, err := ListInstalled(context.Background(), runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 3 {
		t.Errorf("ListInstalled = %d entries, want 3", len(installed))
	}
	if !installed["bash"] || !installed["dpkg"] || !installed["apt"] {
		t.Errorf("expected all three packages to be present in map")
	}
}

func TestListInstalled_empty(t *testing.T) {
	runner := testutil.RunnerReturning([]byte(""), nil)
	installed, err := ListInstalled(context.Background(), runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 0 {
		t.Errorf("ListInstalled = %d entries, want 0", len(installed))
	}
}

func TestListInstalled_trailingNewlineRemoved(t *testing.T) {
	runner := testutil.RunnerReturning([]byte("bash\n"), nil)
	installed, err := ListInstalled(context.Background(), runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 1 {
		t.Errorf("ListInstalled = %d entries, want 1", len(installed))
	}
}

func TestListInstalled_error(t *testing.T) {
	runner := testutil.RunnerReturning(nil, errors.New("dpkg-query failed"))
	_, err := ListInstalled(context.Background(), runner)
	if err == nil {
		t.Error("expected error for failed dpkg-query")
	}
}
