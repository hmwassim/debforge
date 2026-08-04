package appimage

// Install and Remove exercise downloadFunc, runner (install/extract/post
// scripts), and the filesystem. apt calls go through the real aptpty path,
// so coverage here focuses on the appimage-specific flow: version check,
// skip-if-installed, binary install, icon extraction, desktop entry, and
// removal of tracked paths.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer/version"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

func testPkg() *pkg.Package {
	return &pkg.Package{
		Name: "protonup-qt",
		Type: pkg.TypeAppImage,
		URLs: []string{"https://example.com/ProtonUp-Qt-{version}-x86_64.AppImage"},
		AppImg: &pkg.AppImageConfig{
			Bin: "protonup-qt",
			Desktop: &pkg.DesktopEntry{
				ID:         "net.davidotek.pupgui2",
				Name:       "ProtonUp-Qt",
				Icon:       "net.davidotek.pupgui2",
				Categories: "Game;Utility;",
			},
		},
	}
}

// withVerifyServer routes SelectTag's release-asset HEAD verification to an
// httptest server that accepts every tag, so the version machinery doesn't
// hit the real network.
func withVerifyServer(t *testing.T, fn func(srvURL string)) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	old := version.VerifyClient()
	version.SetVerifyClient(srv.Client())
	defer func() { version.SetVerifyClient(old) }()
	fn(srv.URL)
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

func TestInstall_noURL(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeAppImage, AppImg: &pkg.AppImageConfig{Bin: "test"}}
	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error when URL is empty")
	}
}

func TestInstall_shortCircuitsWhenInstalledAndVersionUnchanged(t *testing.T) {
	withVerifyServer(t, func(srvURL string) {
		fs := testutil.NewMockFileSystem()
		fs.Files["/usr/local/bin/protonup-qt"] = []byte("appimage")

		runner := &testutil.MockRunner{
			RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
				return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
			},
		}
		inst := &Installer{runner: runner, fs: fs, sys: &testutil.MockSystem{}}
		p := testPkg()
		p.URLs = []string{srvURL + "/ProtonUp-Qt-{version}-x86_64.AppImage"}
		p.Version = "1.0.0"
		p.Repo = "https://github.com/example/protonup-qt"

		err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
		if err != nil {
			t.Fatalf("expected nil (short-circuit) when installed and version is current, got: %v", err)
		}
	})
}

func TestInstall_proceedsWhenNotInstalledEvenIfVersionUnchanged(t *testing.T) {
	withVerifyServer(t, func(srvURL string) {
		runner := &testutil.MockRunner{
			RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
				return []byte("abc\trefs/tags/v1.0.0\n"), nil, nil
			},
		}
		inst := &Installer{
			runner: runner,
			fs:     testutil.NewMockFileSystem(),
			sys:    &testutil.MockSystem{},
			downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
				return errors.New("no network")
			},
		}
		p := testPkg()
		p.URLs = []string{srvURL + "/ProtonUp-Qt-{version}-x86_64.AppImage"}
		p.Version = "1.0.0"
		p.Repo = "https://github.com/example/protonup-qt"

		err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
		if err == nil {
			t.Fatal("expected an error from the install phase (not a nil short-circuit)")
		}
		if !strings.Contains(err.Error(), "no network") {
			t.Fatalf("expected download stub error, got %v", err)
		}
	})
}

func TestInstall_prereqsError(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			return errors.New("apt install failed")
		},
	}
	p := testPkg()
	p.Packages = []string{"libfuse2t64"}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil || !strings.Contains(err.Error(), "prerequisites") {
		t.Fatalf("expected prerequisites error, got %v", err)
	}
}

func TestInstall_fullFlow(t *testing.T) {
	var runCmds []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			runCmds = append(runCmds, strings.Join(append([]string{name}, args...), " "))
			return nil, nil, nil
		},
	}

	fs := testutil.NewMockFileSystem()
	fs.WalkFunc = func(root string, fn func(path string, info ports.FileInfo, err error) error) error {
		icon := &fileInfo{name: "net.davidotek.pupgui2.svg"}
		return fn("/tmp/x/squashfs-root/net.davidotek.pupgui2.svg", icon, nil)
	}

	inst := &Installer{
		runner: runner,
		fs:     fs,
		sys:    &testutil.MockSystem{},
		downloadFunc: func(_ context.Context, _ ports.FileSystem, url, dest string, _ ports.Spinner, _ string) error {
			if !strings.Contains(url, "ProtonUp-Qt-2.15.1-x86_64.AppImage") {
				t.Errorf("expected version-expanded url, got %q", url)
			}
			return nil
		},
	}

	p := testPkg()
	p.Version = "2.15.1"

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Binary installed to /usr/local/bin via `install -Dm755`.
	if !containsCmd(runCmds, "install -Dm755 ") || !containsCmd(runCmds, " /usr/local/bin/protonup-qt") {
		t.Errorf("expected binary install command, got %v", runCmds)
	}
	// Icon extracted with --appimage-extract and copied into hicolor.
	if !containsCmd(runCmds, "--appimage-extract") {
		t.Errorf("expected --appimage-extract, got %v", runCmds)
	}
	if !containsCmd(runCmds, "scalable/apps/net.davidotek.pupgui2.svg") {
		t.Errorf("expected icon install command, got %v", runCmds)
	}
	// Desktop entry written.
	content, ok := fs.Files["/usr/share/applications/net.davidotek.pupgui2.desktop"]
	if !ok {
		t.Fatal("desktop entry not written")
	}
	desktop := string(content)
	if !strings.Contains(desktop, "Name=ProtonUp-Qt") {
		t.Errorf("desktop missing Name: %q", desktop)
	}
	if !strings.Contains(desktop, "Exec=/usr/local/bin/protonup-qt") {
		t.Errorf("desktop missing Exec: %q", desktop)
	}
	if !strings.Contains(desktop, "Icon=net.davidotek.pupgui2") {
		t.Errorf("desktop missing Icon: %q", desktop)
	}
	if !strings.Contains(desktop, "Categories=Game;Utility;") {
		t.Errorf("desktop missing Categories: %q", desktop)
	}
}

func TestInstall_noDesktop(t *testing.T) {
	var runCmds []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			runCmds = append(runCmds, strings.Join(append([]string{name}, args...), " "))
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{
		runner: runner,
		fs:     fs,
		sys:    &testutil.MockSystem{},
		downloadFunc: func(_ context.Context, _ ports.FileSystem, _, _ string, _ ports.Spinner, _ string) error {
			return nil
		},
	}

	p := testPkg()
	p.AppImg = &pkg.AppImageConfig{Bin: "myapp"}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if containsCmd(runCmds, "--appimage-extract") {
		t.Error("should not extract icon when desktop entry is unset")
	}
	if _, ok := fs.Files["/usr/share/applications/myapp.desktop"]; ok {
		t.Error("should not write desktop entry when desktop block is unset")
	}
	if !containsCmd(runCmds, "/usr/local/bin/myapp") {
		t.Error("binary should still be installed")
	}
}

func TestRemove_fullFlow(t *testing.T) {
	var removed []string
	fs := testutil.NewMockFileSystem()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	var gotArgs []string
	inst := &Installer{
		fs: fs,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}

	p := testPkg()
	p.Remove = []string{"libfuse2t64"}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	want := []string{
		"/usr/local/bin/protonup-qt",
		"/usr/share/applications/net.davidotek.pupgui2.desktop",
		"/usr/share/icons/hicolor/scalable/apps/net.davidotek.pupgui2.svg",
		"/usr/share/icons/hicolor/128x128/apps/net.davidotek.pupgui2.png",
	}
	if len(removed) != len(want) {
		t.Fatalf("removed %v, want %v", removed, want)
	}
	for i := range want {
		if removed[i] != want[i] {
			t.Errorf("removed[%d] = %q, want %q", i, removed[i], want[i])
		}
	}

	aptWant := []string{"remove", "-y", "libfuse2t64"}
	if len(gotArgs) != len(aptWant) {
		t.Fatalf("apt args %v, want %v", gotArgs, aptWant)
	}
	for i := range aptWant {
		if gotArgs[i] != aptWant[i] {
			t.Errorf("apt arg %d: got %q, want %q", i, gotArgs[i], aptWant[i])
		}
	}
}

func TestRemove_noDesktop(t *testing.T) {
	var removed []string
	fs := testutil.NewMockFileSystem()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	inst := &Installer{fs: fs}

	p := testPkg()
	p.AppImg = &pkg.AppImageConfig{Bin: "myapp"}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 1 || removed[0] != "/usr/local/bin/myapp" {
		t.Errorf("removed %v, want only the binary", removed)
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
	if inst.sys == nil {
		t.Error("sys not set")
	}
	if inst.execApt == nil {
		t.Error("execApt should not be nil")
	}
	if inst.downloadFunc == nil {
		t.Error("downloadFunc should not be nil")
	}
}

func TestCheckVersion_firstInstall(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v2.15.1\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "protonup-qt", Repo: "https://github.com/DavidoTek/ProtonUp-Qt.git"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if !updated {
		t.Error("expected updated=true on first install")
	}
	if p.Version != "2.15.1" {
		t.Errorf("expected p.Version=2.15.1, got %q", p.Version)
	}
}

func TestCheckVersion_unchangedNotUpdated(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("abc\trefs/tags/v2.15.1\n"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "protonup-qt", Repo: "https://github.com/DavidoTek/ProtonUp-Qt.git", Version: "2.15.1"}

	updated, err := inst.checkVersion(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if updated {
		t.Error("expected updated=false when the latest tag matches the recorded version")
	}
}

// fileInfo is a minimal ports.FileInfo for the mock Walk.
type fileInfo struct{ name string }

func (f *fileInfo) Name() string { return f.name }
func (f *fileInfo) Size() int64  { return 0 }
func (f *fileInfo) IsDir() bool  { return false }

func containsCmd(cmds []string, substr string) bool {
	for _, c := range cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}
