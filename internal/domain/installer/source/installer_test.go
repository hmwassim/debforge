package source

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
		// Answer both LatestTag's "ls-remote --tags <repo>" and the
		// "ls-remote <repo> HEAD" fallback with a single fake tag, so
		// version detection succeeds without touching the network and
		// without masking the build/install script assertions under test.
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
		return // e.g. "git clone ..." - not a script execution
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

func TestInstall_buildOnlyRunsOnce(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("echo building", "")

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	count := 0
	for _, s := range runner.scripts {
		if s == "echo building" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected build script to run exactly once when no install script is set, ran %d times (all scripts: %v)", count, runner.scripts)
	}
}

func TestInstall_buildAndInstallBothRunOnceInOrder(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("echo building", "echo installing")

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := []string{"echo building", "echo installing"}
	if !equalScripts(runner.scripts, want) {
		t.Errorf("expected scripts %v in order, got %v", want, runner.scripts)
	}
}

func TestInstall_neitherBuildNorInstallRunsNothing(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("", "")

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(runner.scripts) != 0 {
		t.Errorf("expected no scripts run, got %v", runner.scripts)
	}
}

func TestInstall_postinstallAlwaysRunsAfterInstall(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("echo building", "echo installing")
	p.Source.PostinstallScript = "echo postinstalling"

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := []string{"echo building", "echo installing", "echo postinstalling"}
	if !equalScripts(runner.scripts, want) {
		t.Errorf("expected scripts %v in order, got %v", want, runner.scripts)
	}
}

func TestInstall_versionInterpolation(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}

	p := newTestPackage("make VERSION={version}", "")

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// checkVersion (triggered because Repo is set) runs before the build
	// script and may update p.Version from the (fake) latest tag; the
	// interpolated script should reflect whatever p.Version ended up
	// being by the time the build script actually ran, not a value
	// hardcoded ahead of that.
	want := "make VERSION=" + p.Version
	if len(runner.scripts) != 1 || runner.scripts[0] != want {
		t.Errorf("expected interpolated script %q, got %v", want, runner.scripts)
	}
	if p.Version == "" {
		t.Error("expected p.Version to be populated by checkVersion")
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
