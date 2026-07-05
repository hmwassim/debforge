package source

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// versionFallbackRunner fails on "ls-remote --tags" but succeeds on
// "ls-remote <repo> HEAD", triggering the checkVersion fallback path.
type versionFallbackRunner struct {
	recordingRunner
}

func (r *versionFallbackRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name == "git" && len(args) > 0 && args[0] == "ls-remote" && len(args) > 1 && args[1] == "--tags" {
		return nil, nil, errors.New("no tags available")
	}
	return r.recordingRunner.Run(ctx, name, args...)
}

// recordingRunner records every "sh -c <script>" invocation it sees (in
// order) and treats every command as succeeding. "git clone" is special-
// cased so getSource's repo-clone path can complete without touching a
// real network or filesystem.
type recordingRunner struct {
	scripts []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.record(name, args)
	if name == "git" && len(args) > 0 && args[0] == "ls-remote" {
		return []byte("abc123\trefs/tags/v1.0.0\n"), nil, nil
	}
	return nil, nil, nil
}

func (r *recordingRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	r.record(name, args)
	return nil, nil, nil
}

func (r *recordingRunner) record(name string, args []string) {
	if name != "sh" {
		return
	}
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			r.scripts = append(r.scripts, args[i+1])
			return
		}
	}
}

func newTestPackage(build, install string) *pkg.Package {
	return &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Repo: "https://example.invalid/test-src.git",
		Source: &pkg.SourceConfig{
			BuildScript:   build,
			InstallScript: install,
		},
	}
}

func equalScripts(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// gitOKRunner succeeds for git commands (ls-remote, clone) and returns
// nil,nil,nil for everything else (including sh -c via RunWithOptions).
type gitOKRunner struct{}

func (r *gitOKRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name == "git" && len(args) > 0 && args[0] == "ls-remote" {
		return []byte("abc123\trefs/tags/v1.0.0\n"), nil, nil
	}
	return nil, nil, nil
}

func (r *gitOKRunner) RunWithOptions(_ context.Context, _ ports.RunOptions, name string, args ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func TestNewInstaller(t *testing.T) {
	runner := &testutil.MockRunner{}
	fs := testutil.NewMockFileSystem()
	ui := &testutil.MockUI{}
	inst := NewInstaller(runner, fs, ui)
	if inst.runner != runner {
		t.Error("runner not set")
	}
	if inst.fs != fs {
		t.Error("fs not set")
	}
	if inst.ui != ui {
		t.Error("ui not set")
	}
	if inst.execApt == nil {
		t.Error("execApt should not be nil")
	}
}

func TestCheckVersion_gitLsRemoteFallback(t *testing.T) {
	runner := &versionFallbackRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Repo: "https://example.invalid/test-src.git",
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when ls-remote HEAD succeeds")
	}
	if p.Version == "" {
		t.Error("expected p.Version to be set from ls-remote HEAD")
	}
}

func TestInstall_versionInterpolation(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("make VERSION={version}", "")

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := "make VERSION=" + p.Version
	if len(runner.scripts) != 1 || runner.scripts[0] != want {
		t.Errorf("expected interpolated script %q, got %v", want, runner.scripts)
	}
	if p.Version == "" {
		t.Error("expected p.Version to be populated by checkVersion")
	}
}
