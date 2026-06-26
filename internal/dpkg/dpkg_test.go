package dpkg

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestIsInstalled_installed(t *testing.T) {
	if !IsInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), "bash") {
		t.Error("expected installed")
	}
}

func TestIsInstalled_notInstalled(t *testing.T) {
	if IsInstalled(context.Background(), testutil.RunnerReturning([]byte("not-installed\n"), nil), "nonexistent") {
		t.Error("expected not installed")
	}
}

func TestIsInstalled_error(t *testing.T) {
	if IsInstalled(context.Background(), testutil.RunnerReturning(nil, errors.New("dpkg-query failed")), "broken") {
		t.Error("expected not installed on error")
	}
}
