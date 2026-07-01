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
