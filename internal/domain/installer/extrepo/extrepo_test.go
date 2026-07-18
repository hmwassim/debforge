package extrepo

import (
	"context"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- NeedsEnable tests -----------------------------------------------------

func TestNeedsEnable_noFileReturnsTrue(t *testing.T) {
	fs := testutil.NewMockFileSystem()

	got, err := NeedsEnable(context.Background(), "myrepo", fs)
	if err != nil {
		t.Fatalf("NeedsEnable: %v", err)
	}
	if !got {
		t.Error("expected true when no sources file exists")
	}
}

func TestNeedsEnable_enabledReturnsFalse(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/apt/sources.list.d/extrepo_myrepo.sources"] = []byte("Enabled: yes\n")

	got, err := NeedsEnable(context.Background(), "myrepo", fs)
	if err != nil {
		t.Fatalf("NeedsEnable: %v", err)
	}
	if got {
		t.Error("expected false for Enabled: yes")
	}
}

func TestNeedsEnable_disabledReturnsTrue(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/apt/sources.list.d/extrepo_myrepo.sources"] = []byte("Enabled: no\n")

	got, err := NeedsEnable(context.Background(), "myrepo", fs)
	if err != nil {
		t.Fatalf("NeedsEnable: %v", err)
	}
	if !got {
		t.Error("expected true for Enabled: no")
	}
}

func TestNeedsEnable_noEnabledLineDefaultsFalse(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/apt/sources.list.d/extrepo_myrepo.sources"] = []byte("# some comment\nTypes: deb\nURIs: http://example.com\n")

	got, err := NeedsEnable(context.Background(), "myrepo", fs)
	if err != nil {
		t.Fatalf("NeedsEnable: %v", err)
	}
	if got {
		t.Error("expected false when file exists but no Enabled: line (default enabled)")
	}
}

func TestNeedsEnable_invalidRepoNames(t *testing.T) {
	fs := testutil.NewMockFileSystem()

	tests := []struct {
		name string
		repo string
	}{
		{"path traversal with dot-dot", "../evil"},
		{"path traversal with slash", "foo/../../bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NeedsEnable(context.Background(), tt.repo, fs)
			if err != nil {
				t.Fatalf("NeedsEnable: %v", err)
			}
			if got {
				t.Error("expected false for invalid repo name")
			}
		})
	}
}

// ---- Enable tests ----------------------------------------------------------

func TestEnable_runsExtrepoEnable(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	spinner := &testutil.MockSpinner{}

	if err := Enable(context.Background(), "myrepo", runner, spinner); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if len(calls) != 1 || calls[0] != "extrepo enable myrepo" {
		t.Errorf("expected [extrepo enable myrepo], got %v", calls)
	}
	if spinner.Desc != "enabling extrepo myrepo" {
		t.Errorf("expected spinner desc %q, got %q", "enabling extrepo myrepo", spinner.Desc)
	}
}

func TestEnable_extrepoEnableError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, &stubError{"extrepo enable failed"}
		},
	}
	spinner := &testutil.MockSpinner{}

	err := Enable(context.Background(), "myrepo", runner, spinner)
	if err == nil {
		t.Fatal("expected error when extrepo enable fails")
	}
	if !strings.Contains(err.Error(), "enable extrepo myrepo") {
		t.Errorf("expected error to mention enable extrepo, got %v", err)
	}
}

func TestEnable_doesNotRunAptGetUpdate(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	spinner := &testutil.MockSpinner{}

	if err := Enable(context.Background(), "myrepo", runner, spinner); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	for _, c := range calls {
		if strings.Contains(c, "apt-get") {
			t.Errorf("Enable should not run apt-get update, got: %s", c)
		}
	}
}

// stubError is a minimal error type for testing.
type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }
