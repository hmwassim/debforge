package apt

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestFindInstalledConflicts(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if len(args) >= 3 && args[2] == "pkg-a" {
				return []byte("installed"), nil, nil
			}
			return []byte("not-installed"), nil, nil
		},
	}

	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b", "pkg-c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "pkg-a" {
		t.Errorf("FindInstalledConflicts = %v, want [pkg-a]", got)
	}
}

func TestFindInstalledConflicts_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("not-installed"), nil, nil
		},
	}

	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts = %v, want []", got)
	}
}

func TestFindInstalledConflicts_empty(t *testing.T) {
	runner := &testutil.MockRunner{}
	got, err := FindInstalledConflicts(context.Background(), runner, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts(nil) = %v, want []", got)
	}
}

func TestFindInstalledConflicts_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, context.DeadlineExceeded
		},
	}
	got, err := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a"})
	if err == nil {
		t.Fatal("expected error from FindInstalledConflicts")
	}
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts with error = %v, want []", got)
	}
}
