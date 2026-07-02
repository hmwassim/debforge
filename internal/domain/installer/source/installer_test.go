package source

import (
	"context"
	"errors"
	"strings"
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

func TestRemove_removeScript(t *testing.T) {
	runner := &recordingRunner{}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Source: &pkg.SourceConfig{
			RemoveScript: "echo removing",
		},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.scripts) != 1 || runner.scripts[0] != "echo removing" {
		t.Errorf("expected remove script to run, got %v", runner.scripts)
	}
}

func TestRemove_aptGetRemove(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		fs: testutil.NewMockFileSystem(),
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Remove: []string{"pkg-a", "pkg-b"},
		Source: &pkg.SourceConfig{},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "pkg-a", "pkg-b"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_bothScriptAndApt(t *testing.T) {
	runner := &recordingRunner{}
	var gotArgs []string
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Remove: []string{"pkg-a"},
		Source: &pkg.SourceConfig{
			RemoveScript: "echo removing",
		},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.scripts) != 1 || runner.scripts[0] != "echo removing" {
		t.Errorf("expected remove script to run, got %v", runner.scripts)
	}
	want := []string{"remove", "-y", "pkg-a"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
}

func TestRemove_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeDeb}
	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRemove_noScriptNoPackages(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{Name: "test-src", Type: pkg.TypeSource, Source: &pkg.SourceConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestGetSource_gitClone(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name)
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Repo:   "https://example.com/repo.git",
		Source: &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}
	if len(calls) == 0 || calls[0] != "git" {
		t.Errorf("expected git clone, got %v", calls)
	}
}

func TestGetSource_gitCloneWithVersion(t *testing.T) {
	var recordedArgs []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recordedArgs = append(recordedArgs, name)
			recordedArgs = append(recordedArgs, args...)
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:    "test-src",
		Repo:    "https://example.com/repo.git",
		Version: "1.0.0",
		Source:  &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}
	if len(recordedArgs) < 7 || recordedArgs[4] != "--branch" || recordedArgs[5] != "v1.0.0" {
		t.Errorf("expected --branch v1.0.0 in git clone args, got %v", recordedArgs)
	}
}

func TestGetSource_gitCloneSkipCloneNoURL(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Repo:   "https://example.com/repo.git",
		Source: &pkg.SourceConfig{SkipClone: true},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when SkipClone is set but no URL")
	}
}

func TestGetSource_noRepoNoURL(t *testing.T) {
	inst := &Installer{fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:   "test-src",
		Type:   pkg.TypeSource,
		Repo:   "",
		URL:    "",
		Source: &pkg.SourceConfig{},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when neither repo nor URL is set")
	}
}

func TestGetSource_downloadTar(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			if name == "tar" && len(args) >= 2 && args[0] == "tf" {
				return []byte("usr/bin/hello\nusr/share/man/hello.1\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URL:  "https://example.com/test-src-{version}.tar.gz",
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	var extractArgs []string
	for _, cmd := range recorded {
		if cmd[0] == "tar" && len(cmd) > 1 && cmd[1] == "-xf" {
			extractArgs = cmd
			break
		}
	}
	if extractArgs == nil {
		t.Fatal("expected tar -xf extract call")
	}
	hasStrip := false
	hasC := false
	for _, a := range extractArgs {
		if strings.Contains(a, "--strip-components") {
			hasStrip = true
		}
		if a == "-C" {
			hasC = true
		}
	}
	if !hasStrip {
		t.Error("expected --strip-components=1 in tar extract args when archive has top-level dir")
	}
	if !hasC {
		t.Error("expected -C in tar extract args")
	}
}

func TestGetSource_downloadTar_noTopDir(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			if name == "tar" && len(args) >= 2 && args[0] == "tf" {
				return []byte("hello\nhello.1\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URL:  "https://example.com/test-src.tar.gz",
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	for _, cmd := range recorded {
		if cmd[0] == "tar" && len(cmd) > 1 && cmd[1] == "-xf" {
			for _, a := range cmd {
				if strings.Contains(a, "--strip-components") {
					t.Error("did not expect --strip-components when archive has no top-level dir")
				}
			}
		}
	}
}

func TestGetSource_downloadZip(t *testing.T) {
	var recorded [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			recorded = append(recorded, append([]string{name}, args...))
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		fs:     testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URL:  "https://example.com/test-src-{version}.zip",
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("getSource: %v", err)
	}

	foundUnzip := false
	for _, cmd := range recorded {
		if cmd[0] == "unzip" {
			foundUnzip = true
			break
		}
	}
	if !foundUnzip {
		t.Errorf("expected unzip command, got %v", recorded)
	}
}

func TestGetSource_downloadError(t *testing.T) {
	inst := &Installer{
		fs: testutil.NewMockFileSystem(),
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return errors.New("download failed")
		},
	}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		URL:  "https://example.com/test-src.tar.gz",
		Source: &pkg.SourceConfig{
			BuildScript: "echo built",
		},
	}

	_, err := inst.getSource(context.Background(), p, "/tmp/x", &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when download fails")
	}
}

func TestInstall_checkVersionCmdError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("version command failed")
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:        "test-src",
		Type:        pkg.TypeSource,
		VersionCmd:  "bad-command --version",
		Repo:        "https://example.com/repo.git",
		Source:      &pkg.SourceConfig{},
		ForceInstall: true,
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when version command fails")
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
