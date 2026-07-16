package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if inst.tagRefsCache == nil {
		t.Error("tagRefsCache should not be nil")
	}
	if inst.headCache == nil {
		t.Error("headCache should not be nil")
	}
}

func TestCheckVersion_cacheHit(t *testing.T) {
	callCount := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "git" && len(args) > 0 && args[0] == "ls-remote" {
				callCount++
				return []byte("abc123\trefs/tags/v2.0.0\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem(), tagRefsCache: make(map[string][]string), headCache: make(map[string]string)}

	p1 := &pkg.Package{
		Name:      "font-a",
		Type:      pkg.TypeSource,
		Repo:      "https://example.invalid/nerd-fonts.git",
		TagPrefix: "v",
		Source:    &pkg.SourceConfig{BuildScript: "echo build"},
	}
	p2 := &pkg.Package{
		Name:      "font-b",
		Type:      pkg.TypeSource,
		Repo:      "https://example.invalid/nerd-fonts.git",
		TagPrefix: "v",
		Source:    &pkg.SourceConfig{BuildScript: "echo build"},
	}

	_, err := inst.checkVersion(context.Background(), p1, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("first checkVersion: %v", err)
	}
	if p1.Version != "2.0.0" {
		t.Errorf("expected p1.Version=2.0.0, got %q", p1.Version)
	}

	_, err = inst.checkVersion(context.Background(), p2, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("second checkVersion: %v", err)
	}
	if p2.Version != "2.0.0" {
		t.Errorf("expected p2.Version=2.0.0, got %q", p2.Version)
	}

	if callCount != 1 {
		t.Errorf("expected git ls-remote called once (cache hit), got %d", callCount)
	}
}

func TestCheckVersion_cacheHonorsPerPackageVersionCmd(t *testing.T) {
	versionCmdCalls := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "git" {
				return []byte("abc123\trefs/tags/v2.0.0\n"), nil, nil
			}
			if name == "sh" {
				versionCmdCalls++
				return []byte("9.9.9\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem(), tagRefsCache: make(map[string][]string), headCache: make(map[string]string)}

	p1 := &pkg.Package{
		Name:   "font-a",
		Type:   pkg.TypeSource,
		Repo:   "https://example.invalid/shared.git",
		Source: &pkg.SourceConfig{BuildScript: "echo build"},
	}
	p2 := &pkg.Package{
		Name:       "special-b",
		Type:       pkg.TypeSource,
		Repo:       "https://example.invalid/shared.git",
		VersionCmd: "echo 9.9.9",
		Source:     &pkg.SourceConfig{BuildScript: "echo build"},
	}

	if _, err := inst.checkVersion(context.Background(), p1, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkVersion p1: %v", err)
	}
	if _, err := inst.checkVersion(context.Background(), p2, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkVersion p2: %v", err)
	}

	if p2.Version != "9.9.9" {
		t.Errorf("expected p2.Version=9.9.9 (from its own VersionCmd), got %q", p2.Version)
	}
	if versionCmdCalls == 0 {
		t.Error("expected p2's VersionCmd to run, it never did")
	}
}

func TestCheckVersion_cacheHonorsPerPackageVerifyURL(t *testing.T) {
	existsFor := map[string]bool{
		"a-2.0.0": true, "a-1.0.0": true,
		"b-1.0.0": true,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if existsFor[strings.TrimPrefix(r.URL.Path, "/")] {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "git" && len(args) > 0 && args[0] == "ls-remote" {
				return []byte("a\trefs/tags/v1.0.0\nb\trefs/tags/v2.0.0\n"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem(), tagRefsCache: make(map[string][]string), headCache: make(map[string]string)}

	p1 := &pkg.Package{
		Name:   "font-a",
		Type:   pkg.TypeSource,
		Repo:   "https://example.invalid/shared.git",
		URLs:   []string{srv.URL + "/a-{version}"},
		Source: &pkg.SourceConfig{BuildScript: "echo build"},
	}
	if _, err := inst.checkVersion(context.Background(), p1, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkVersion p1: %v", err)
	}
	if p1.Version != "2.0.0" {
		t.Fatalf("expected p1.Version=2.0.0, got %q", p1.Version)
	}

	p2 := &pkg.Package{
		Name:   "font-b",
		Type:   pkg.TypeSource,
		Repo:   "https://example.invalid/shared.git",
		URLs:   []string{srv.URL + "/b-{version}"},
		Source: &pkg.SourceConfig{BuildScript: "echo build"},
	}
	if _, err := inst.checkVersion(context.Background(), p2, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkVersion p2: %v", err)
	}
	if p2.Version != "1.0.0" {
		t.Errorf("expected p2.Version=1.0.0 (its 2.0.0 asset 404s), got %q", p2.Version)
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
