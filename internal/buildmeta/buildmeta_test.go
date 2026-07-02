package buildmeta

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestDeriveVersion(t *testing.T) {
	runner := testutil.RunnerReturning([]byte("v1.2.3\n"), nil)
	v := DeriveVersion(context.Background(), runner, "/src")
	if v != "v1.2.3" {
		t.Errorf("DeriveVersion = %q, want %q", v, "v1.2.3")
	}
}

func TestDeriveVersion_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("git error")
		},
	}
	v := DeriveVersion(context.Background(), runner, "/src")
	if v != DefaultVersion {
		t.Errorf("DeriveVersion = %q, want %q", v, DefaultVersion)
	}
}

func TestDeriveVersion_emptyOutput(t *testing.T) {
	runner := testutil.RunnerReturning([]byte(""), nil)
	v := DeriveVersion(context.Background(), runner, "/src")
	if v != DefaultVersion {
		t.Errorf("DeriveVersion = %q, want %q", v, DefaultVersion)
	}
}

func TestDeriveVersion_whitespaceOnly(t *testing.T) {
	runner := testutil.RunnerReturning([]byte("   \n"), nil)
	v := DeriveVersion(context.Background(), runner, "/src")
	if v != DefaultVersion {
		t.Errorf("DeriveVersion = %q, want %q", v, DefaultVersion)
	}
}

func TestLdflags(t *testing.T) {
	s := Ldflags("v1.0.0")
	if s != "-X main.version=v1.0.0" {
		t.Errorf("Ldflags = %q, want %q", s, "-X main.version=v1.0.0")
	}
}
