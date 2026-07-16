package apt

import (
	"context"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestEnableExtrepos_enablesAndUpdates(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{runner: runner, fs: fs}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	want := []string{"extrepo enable myrepo", "apt-get update"}
	for i, c := range want {
		if i >= len(calls) || calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestEnableExtrepos_noRepos(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands when no extrepos")
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
}

func TestEnableExtrepos_alreadyEnabled(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name)
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/apt/sources.list.d/extrepo_myrepo.sources"] = []byte("Enabled: yes\n")
	inst := &Installer{runner: runner, fs: fs}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("expected no calls when all repos already enabled, got %v", calls)
	}
}

func TestDisableExtrepos_disablesEachRepo(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{Extrepo: []string{"repo-a"}}}

	if err := inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("disableExtrepos: %v", err)
	}

	want := []string{"extrepo disable repo-a"}
	for i, c := range want {
		if i >= len(calls) || calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestDisableExtrepos_multipleRepos(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{Extrepo: []string{"repo-a", "repo-b"}}}

	if err := inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("disableExtrepos: %v", err)
	}

	want := []string{"extrepo disable repo-a", "extrepo disable repo-b"}
	if len(calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %v", len(calls), len(want), calls)
	}
	for i, c := range want {
		if calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestDisableExtrepos_noRepos(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands")
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{}}

	if err := inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("disableExtrepos: %v", err)
	}
}
