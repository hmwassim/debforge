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

func TestInstall_prereqsError(t *testing.T) {
	inst := &Installer{
		runner: &gitOKRunner{},
		fs:     testutil.NewMockFileSystem(),
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			return errors.New("apt install failed")
		},
	}
	p := &pkg.Package{
		Name:     "test-src",
		Type:     pkg.TypeSource,
		Repo:     "https://example.com/repo.git",
		Packages: []string{"build-dep"},
		Source:   &pkg.SourceConfig{},
	}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "apt install") {
		t.Fatalf("expected prereqs error, got %v", err)
	}
}

func TestInstall_buildScriptError(t *testing.T) {
	var buildCalled bool
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "git" {
				return []byte("abc123\trefs/tags/v1.0.0\n"), nil, nil
			}
			return nil, nil, nil
		},
		RunWithOptionsFunc: func(_ context.Context, _ ports.RunOptions, _ string, _ ...string) ([]byte, []byte, error) {
			buildCalled = true
			return nil, nil, errors.New("build failed")
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Repo: "https://example.com/repo.git",
		Source: &pkg.SourceConfig{
			BuildScript: "echo building",
		},
	}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "building") {
		t.Fatalf("expected build error, got %v", err)
	}
	if !buildCalled {
		t.Error("RunWithOptions was never called")
	}
}

func TestInstall_installScriptError(t *testing.T) {
	var callCount int
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "git" {
				return []byte("abc123\trefs/tags/v1.0.0\n"), nil, nil
			}
			return nil, nil, nil
		},
		RunWithOptionsFunc: func(_ context.Context, _ ports.RunOptions, _ string, _ ...string) ([]byte, []byte, error) {
			callCount++
			if callCount == 2 {
				return nil, nil, errors.New("install failed")
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name: "test-src",
		Type: pkg.TypeSource,
		Repo: "https://example.com/repo.git",
		Source: &pkg.SourceConfig{
			BuildScript:   "echo building",
			InstallScript: "echo installing",
		},
	}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "installing") {
		t.Fatalf("expected install error, got %v", err)
	}
}

func TestInstall_postinstallError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "git" {
				return []byte("abc123\trefs/tags/v1.0.0\n"), nil, nil
			}
			if name == "sh" {
				return nil, nil, errors.New("postinstall failed")
			}
			return nil, nil, nil
		},
		RunWithOptionsFunc: func(_ context.Context, _ ports.RunOptions, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := newTestPackage("echo building", "")
	p.Source.PostinstallScript = "echo postinstalling"
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "post-install") {
		t.Fatalf("expected postinstall error, got %v", err)
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
		Name:         "test-src",
		Type:         pkg.TypeSource,
		VersionCmd:   "bad-command --version",
		Repo:         "https://example.com/repo.git",
		Source:       &pkg.SourceConfig{},
		ForceInstall: true,
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error when version command fails")
	}
}
