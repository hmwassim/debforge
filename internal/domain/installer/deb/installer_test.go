package deb

// Install is a thin wrapper over Prepare + execApt + Finalize; Remove
// calls execApt and post-remove scripts. Both depend on aptpty's real
// pty-based runner, so meaningful coverage lives in Prepare and Finalize.
// The tests below exercise checkVersion, Prepare (including .deb suffix
// handling via an injected DownloadFunc), Finalize, and Abort.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer/version"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestCheckVersion_firstInstall(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true on first install (no prior recorded version)")
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected p.Version=1.0.0, got %q", p.Version)
	}
}

func TestCheckVersion_unchangedNotUpdated(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git", Version: "1.0.0"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if updated {
		t.Error("expected updated=false when the latest tag matches the recorded version")
	}
}

func TestCheckVersion_newerTagIsUpdated(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v2.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", Repo: "https://github.com/o/p.git", Version: "1.0.0"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when a newer tag is available")
	}
	if p.Version != "2.0.0" {
		t.Errorf("expected p.Version updated to 2.0.0, got %q", p.Version)
	}
}

func TestCheckVersion_versionCmd(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("9.9.9\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", VersionCmd: "echo 9.9.9"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated || p.Version != "9.9.9" {
		t.Errorf("expected updated=true and p.Version=9.9.9, got updated=%v p.Version=%q", updated, p.Version)
	}
}

func TestCheckVersion_runnerErrorPropagates(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("boom")
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-deb", VersionCmd: "false"}

	if _, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Error("expected an error to propagate from a failing version command")
	}
}

func TestInstall_shortCircuitsWhenInstalledAndVersionUnchanged(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	old := version.VerifyClient()
	version.SetVerifyClient(srv.Client())
	defer func() { version.SetVerifyClient(old) }()

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return []byte("installed\n"), nil, nil
			}
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem()}
	p := &pkg.Package{
		Name:    "test-deb",
		Type:    pkg.TypeDeb,
		URLs:    []string{srv.URL + "/{version}.deb"},
		Version: "1.0.0",
		Repo:    "https://github.com/o/p.git",
		Deb:     &pkg.DebConfig{Package: "test-deb"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("expected nil (short-circuit) when installed and version is current, got: %v", err)
	}
}

func TestInstall_noURL(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test-deb", Type: pkg.TypeDeb}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error when URL is empty")
	}
}

func TestRemove_noPackages(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test-deb", Type: pkg.TypeDeb}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestRemove_fallsBackToDebPackage(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-deb", Type: pkg.TypeDeb, Deb: &pkg.DebConfig{Package: "mydeb"}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "mydeb"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestNewInstaller(t *testing.T) {
	runner := &testutil.MockRunner{}
	fs := testutil.NewMockFileSystem()
	ui := &testutil.MockUI{}
	inst := NewInstaller(runner, fs, ui, &testutil.MockSystem{})
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

func TestInstall_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeApt}
	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRemove_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeApt}
	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRemove_callsExecApt(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-deb", Type: pkg.TypeDeb, Packages: []string{"pkg-a"}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "pkg-a"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestInstall_versionlessNoPrereqsFailsAtDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inst := &Installer{fs: testutil.NewMockFileSystem(), downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
		return errors.New("no network")
	}}
	p := &pkg.Package{
		Name: "test-deb",
		Type: pkg.TypeDeb,
		URLs: []string{srv.URL + "/pkg.deb"},
		Deb:  &pkg.DebConfig{Package: "test-deb"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "no network") {
		t.Fatalf("expected download stub error, got %v", err)
	}
}

func TestInstall_prereqsError(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			return errors.New("apt install failed")
		},
	}
	p := &pkg.Package{
		Name:     "test-deb",
		Type:     pkg.TypeDeb,
		URLs:     []string{"http://example.com/pkg.deb"},
		Packages: []string{"prereq"},
	}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "prerequisites") {
		t.Fatalf("expected prerequisites error, got %v", err)
	}
}

func TestInstall_proceedsWhenNotInstalledEvenIfVersionUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, errors.New("package not installed")
			}
			return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: testutil.NewMockFileSystem(), downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
		return errors.New("no network")
	}}
	p := &pkg.Package{
		Name:    "test-deb",
		Type:    pkg.TypeDeb,
		URLs:    []string{srv.URL + "/{version}.deb"},
		Version: "1.0.0",
		Repo:    "https://github.com/o/p.git",
		Deb:     &pkg.DebConfig{Package: "test-deb"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected an error from the install phase (not a nil short-circuit)")
	}
	if strings.Contains(err.Error(), "up to date") {
		t.Error("install short-circuited due to version match when package is not on the system")
	}
}

func TestFinalize_removesTempDir(t *testing.T) {
	var removed []string
	fs := testutil.NewMockFileSystem()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	inst := &Installer{runner: &testutil.MockRunner{}, fs: fs, tempDirs: map[string]string{
		"test-deb": "/tmp/debforge-test-abc",
	}}

	if err := inst.Finalize(context.Background(), &pkg.Package{Name: "test-deb"}, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if len(removed) != 1 || removed[0] != "/tmp/debforge-test-abc" {
		t.Errorf("expected temp dir removed, got %v", removed)
	}
	if _, ok := inst.tempDirs["test-deb"]; ok {
		t.Error("expected tempDirs entry to be deleted after Finalize")
	}
}

func TestAbort_removesTempDirWithoutPostInstall(t *testing.T) {
	var removed []string
	var postInstallRan bool
	fs := testutil.NewMockFileSystem()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "sh" {
				postInstallRan = true
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner, fs: fs, tempDirs: map[string]string{
		"test-deb": "/tmp/debforge-test-xyz",
	}}

	inst.Abort(&pkg.Package{Name: "test-deb", PostInstall: "echo post"})

	if len(removed) != 1 || removed[0] != "/tmp/debforge-test-xyz" {
		t.Errorf("expected temp dir removed, got %v", removed)
	}
	if postInstallRan {
		t.Error("Abort should not run postinstall scripts")
	}
	if _, ok := inst.tempDirs["test-deb"]; ok {
		t.Error("expected tempDirs entry to be deleted after Abort")
	}
}

func TestAbort_noopWhenNoTempDir(t *testing.T) {
	inst := &Installer{runner: &testutil.MockRunner{}, fs: testutil.NewMockFileSystem()}
	inst.Abort(&pkg.Package{Name: "test-deb"})
}

func TestPrepare_debSuffixHandling(t *testing.T) {
	tests := []struct {
		name     string
		urlPath  string
		expected string
	}{
		{"no extension (Discord)", "/api/download?platform=linux&format=deb", "download.deb"},
		{"already .deb", "/releases/v1.0/pkg.deb", "pkg.deb"},
		{"uppercase .DEB", "/releases/v1.0/pkg.DEB", "pkg.DEB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := testutil.NewMockFileSystem()
			var capturedDest string
			stub := func(_ context.Context, _ ports.FileSystem, _, dest string, _ ports.Spinner, _ string) error {
				capturedDest = dest
				return errors.New("stub: skip download")
			}
			inst := &Installer{
				runner:       &testutil.MockRunner{},
				fs:           fs,
				execApt:      func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error { return nil },
				downloadFunc: stub,
			}

			p := &pkg.Package{
				Name: "test-deb",
				Type: pkg.TypeDeb,
				URLs: []string{tt.urlPath},
			}

			_, err := inst.Prepare(context.Background(), p, &testutil.MockSpinner{})
			if err == nil || !strings.Contains(err.Error(), "stub: skip download") {
				t.Fatalf("expected stub download error, got %v", err)
			}

			got := filepath.Base(capturedDest)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}
