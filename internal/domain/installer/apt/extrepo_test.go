package apt

import (
	"context"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// mockExtrepoManager records calls for testing.
type mockExtrepoManager struct {
	needsEnableFn func(ctx context.Context, repo string) (bool, error)
	enableFn      func(ctx context.Context, repo string, spinner ports.Spinner) error
}

func (m *mockExtrepoManager) NeedsEnable(ctx context.Context, repo string) (bool, error) {
	if m.needsEnableFn != nil {
		return m.needsEnableFn(ctx, repo)
	}
	return true, nil
}

func (m *mockExtrepoManager) Enable(ctx context.Context, repo string, spinner ports.Spinner) error {
	if m.enableFn != nil {
		return m.enableFn(ctx, repo, spinner)
	}
	return nil
}

func TestEnableExtrepos_enablesAndUpdates(t *testing.T) {
	var runnerCalls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			runnerCalls = append(runnerCalls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	var extCalls []string
	ext := &mockExtrepoManager{
		enableFn: func(_ context.Context, repo string, _ ports.Spinner) error {
			extCalls = append(extCalls, repo)
			return nil
		},
	}
	inst := &Installer{runner: runner, fs: fs, extrepo: ext}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	if len(extCalls) != 1 || extCalls[0] != "myrepo" {
		t.Errorf("extrepo.Enable calls: got %v, want [myrepo]", extCalls)
	}
	if len(runnerCalls) != 1 || runnerCalls[0] != "apt-get update" {
		t.Errorf("runner calls: got %v, want [apt-get update]", runnerCalls)
	}
}

func TestEnableExtrepos_noRepos(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands when no extrepos")
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, extrepo: &mockExtrepoManager{}}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
}

func TestEnableExtrepos_alreadyEnabled(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	}
	ext := &mockExtrepoManager{
		needsEnableFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil // already enabled
		},
	}
	inst := &Installer{runner: runner, extrepo: ext}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	// Runner should not be called — no update needed
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
