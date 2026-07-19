package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer/extrepo"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type mockSpinner struct{ desc string }

func (m *mockSpinner) Done()            {}
func (m *mockSpinner) Fail()            {}
func (m *mockSpinner) DoneWarn()        {}
func (m *mockSpinner) DoneInfo()        {}
func (m *mockSpinner) Pause()           {}
func (m *mockSpinner) Resume()          {}
func (m *mockSpinner) Stop()            {}
func (m *mockSpinner) SetDesc(d string) { m.desc = d }

type variantRecorder struct {
	variants   []string
	forceFlags []bool
}

func (r *variantRecorder) Install(_ context.Context, p *pkg.Package, _ ports.Spinner) error {
	r.forceFlags = append(r.forceFlags, p.ForceInstall)
	if p.Apt != nil {
		r.variants = append(r.variants, p.Apt.Variant)
	}
	return nil
}

func (r *variantRecorder) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return nil
}

// errorRecorder returns removeErr on Remove; Install delegates to
// variantRecorder.
type errorRecorder struct {
	variantRecorder
	removeErr error
}

func (r *errorRecorder) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return r.removeErr
}

type nopRunner struct{}

func (n *nopRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}
func (n *nopRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return nil, nil, nil
}

type successRunner struct{}

func (r *successRunner) Run(_ context.Context, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
}
func (r *successRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) (stdout, stderr []byte, err error) {
	return []byte("installed\n"), nil, nil
}

// dpkgRunner simulates dpkg-query responses for a fixed set of installed
// packages. It returns "installed" for Status-Status queries and a
// newline-separated package list for ${Package} queries.
type dpkgRunner struct {
	installed []string
}

func (r *dpkgRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	for _, a := range args {
		if strings.Contains(a, "${db:Status-Status}") {
			return []byte("installed\n"), nil, nil
		}
	}
	// dpkg-query -W -f ${Package}\n
	list := strings.Join(r.installed, "\n") + "\n"
	return []byte(list), nil, nil
}
func (r *dpkgRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// failOnDpkgRunner fails on dpkg-query but passes all other commands.
type failOnDpkgRunner struct {
	called    bool
	callCount int
}

func (r *failOnDpkgRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name == "dpkg-query" {
		r.called = true
		r.callCount++
		return nil, nil, errors.New("dpkg error")
	}
	return []byte("installed\n"), nil, nil
}

func (r *failOnDpkgRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, _ string, _ ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func newStateManagerForTest(t *testing.T) (*StateManager, string, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "debforge-test-*.json")
	if err != nil {
		t.Fatalf("create temp state: %v", err)
	}
	if _, err := tmpFile.Write([]byte("{}\n")); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("write initial state: %v", err)
	}
	tmpFile.Close()
	stateStore := store.NewStore[State](fs.NewFileSystem(), tmpFile.Name())
	stateSvc := NewStateManager(stateStore)
	return stateSvc, tmpFile.Name(), func() { os.Remove(tmpFile.Name()) }
}

// nopAptUpdater is a no-op apt updater for tests.
type nopAptUpdater struct{}

func (nopAptUpdater) RunUpdate(_ context.Context, _ ports.Spinner) error { return nil }

// nopExtrepoManager is a no-op extrepo manager for tests.
type nopExtrepoManager struct{}

func (nopExtrepoManager) NeedsEnable(_ context.Context, _ string) (bool, error) { return false, nil }
func (nopExtrepoManager) Enable(_ context.Context, _ string, _ ports.Spinner) error { return nil }

// nopPackageLister is a no-op package lister for tests.
type nopPackageLister struct{}

func (nopPackageLister) ListInstalled(_ context.Context) (map[string]bool, error) {
	return make(map[string]bool), nil
}

// testExtrepoManager delegates to the real extrepo package using the runner and fs.
type testExtrepoManager struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
}

func (m *testExtrepoManager) NeedsEnable(ctx context.Context, repo string) (bool, error) {
	return extrepo.NeedsEnable(ctx, repo, m.fs)
}

func (m *testExtrepoManager) Enable(ctx context.Context, repo string, spinner ports.Spinner) error {
	return extrepo.Enable(ctx, repo, m.runner, spinner)
}

// testAptUpdater delegates to aptpty.RunUpdate using the runner.
type testAptUpdater struct {
	runner ports.CommandRunner
}

func (m *testAptUpdater) RunUpdate(ctx context.Context, spinner ports.Spinner) error {
	return aptpty.RunUpdate(ctx, m.runner, spinner)
}

// testPackageLister delegates to dpkg.ListInstalled using the runner.
type testPackageLister struct {
	runner ports.CommandRunner
}

func (m *testPackageLister) ListInstalled(ctx context.Context) (map[string]bool, error) {
	return dpkg.ListInstalled(ctx, m.runner)
}
