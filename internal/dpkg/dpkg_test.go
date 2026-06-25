package dpkg

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestIsInstalled_installed(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed\n"), nil, nil
		},
	}
	if !IsInstalled(context.Background(), runner, "bash") {
		t.Error("expected installed")
	}
}

func TestIsInstalled_notInstalled(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("not-installed\n"), nil, nil
		},
	}
	if IsInstalled(context.Background(), runner, "nonexistent") {
		t.Error("expected not installed")
	}
}

func TestIsInstalled_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("dpkg-query failed")
		},
	}
	if IsInstalled(context.Background(), runner, "broken") {
		t.Error("expected not installed on error")
	}
}
