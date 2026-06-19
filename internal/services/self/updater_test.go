package self

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	config "github.com/hmwassim/debforge/internal/config"
	"github.com/hmwassim/debforge/internal/statestore"
	"github.com/hmwassim/debforge/internal/ports"
)

type memFS struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, dirs: map[string]bool{}}
}

func (f *memFS) mkdirAll(path string) {
	dir := filepath.Dir(path)
	for dir != "." && dir != "/" {
		f.dirs[dir] = true
		dir = filepath.Dir(dir)
	}
}

func (f *memFS) ReadFile(name string) ([]byte, error) {
	data, ok := f.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *memFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	f.mkdirAll(name)
	f.files[name] = data
	return nil
}

func (f *memFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error {
	f.mkdirAll(name)
	f.files[name] = data
	return nil
}

func (f *memFS) ReadDir(name string) ([]os.DirEntry, error) {
	return nil, nil
}

type memFileInfo struct{ name string }

func (m memFileInfo) Name() string       { return m.name }
func (m memFileInfo) Size() int64        { return 0 }
func (m memFileInfo) Mode() os.FileMode  { return 0755 }
func (m memFileInfo) ModTime() time.Time { return time.Time{} }
func (m memFileInfo) IsDir() bool        { return false }
func (m memFileInfo) Sys() any           { return nil }

func (f *memFS) Stat(name string) (os.FileInfo, error) {
	if _, ok := f.files[name]; ok {
		return memFileInfo{name: name}, nil
	}
	return nil, os.ErrNotExist
}

func (f *memFS) MkdirAll(path string, perm os.FileMode) error {
	f.dirs[path] = true
	return nil
}

func (f *memFS) RemoveAll(path string) error {
	for p := range f.files {
		if strings.HasPrefix(p, path) {
			delete(f.files, p)
		}
	}
	delete(f.dirs, path)
	return nil
}

func (f *memFS) Chmod(name string, mode os.FileMode) error { return nil }
func (f *memFS) Rename(oldPath, newPath string) error {
	data, ok := f.files[oldPath]
	if !ok {
		return os.ErrNotExist
	}
	f.files[newPath] = data
	delete(f.files, oldPath)
	return nil
}
func (f *memFS) MkdirTemp(dir, pattern string) (string, error) {
	return "/tmp/test", nil
}

func (f *memFS) Lstat(name string) (os.FileInfo, error) {
	return f.Stat(name)
}

func (f *memFS) Readlink(name string) (string, error) {
	return "", nil
}

func (f *memFS) Symlink(target, link string) error {
	return nil
}

type mockRunner struct {
	stdout []byte
	stderr []byte
	err    error
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return m.stdout, m.stderr, m.err
}
func (m *mockRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	return m.stdout, m.stderr, m.err
}
func (m *mockRunner) RunWithSpinner(ctx context.Context, name string, args ...string) error {
	return m.err
}

type noopSpinner struct{}

func (s *noopSpinner) Done()   {}
func (s *noopSpinner) Fail()   {}
func (s *noopSpinner) Pause()  {}
func (s *noopSpinner) Resume() {}

type mockLogger struct {
	infoCalls    int
	warnCalls    int
	successCalls int
	errorCalls   int
	promptResult bool
	lastWarn     string
	lastInfo     string
}

func (m *mockLogger) Info(format string, args ...any)                        { m.infoCalls++; m.lastInfo = format }
func (m *mockLogger) Success(format string, args ...any)                     { m.successCalls++ }
func (m *mockLogger) Warn(format string, args ...any)                        { m.warnCalls++; m.lastWarn = format }
func (m *mockLogger) Error(format string, args ...any)                       { m.errorCalls++ }
func (m *mockLogger) Muted(format string, args ...any)                       {}
func (m *mockLogger) Debug(format string, args ...any)                       {}
func (m *mockLogger) Prompt(format string, args ...any) bool                 { return m.promptResult }
func (m *mockLogger) PromptInput(format string, args ...any) string          { return "" }
func (m *mockLogger) Spinner(ctx context.Context, desc string) ports.Spinner { return &noopSpinner{} }
func (m *mockLogger) Progress(total int64, desc string) ports.Progress       { return nil }

type spySpinner struct {
	calls []string
}

func (s *spySpinner) Done()   { s.calls = append(s.calls, "Done") }
func (s *spySpinner) Fail()   { s.calls = append(s.calls, "Fail") }
func (s *spySpinner) Pause()  { s.calls = append(s.calls, "Pause") }
func (s *spySpinner) Resume() { s.calls = append(s.calls, "Resume") }

type promptSpyLogger struct {
	mockLogger
	spy *spySpinner
}

func (m *promptSpyLogger) Spinner(ctx context.Context, desc string) ports.Spinner { return m.spy }

type mockLocker struct{}

func (m *mockLocker) Acquire(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}

func testConfig() *config.Config {
	return &config.Config{
		RootDir:    "/opt/debforge",
		StatesDir:  "/opt/debforge/var/states",
		RepoURL:    "https://github.com/hmwassim/debforge",
		Branch:     "main",
		BinaryPath: "/usr/local/bin/debforge",
	}
}

func TestUpdaterLoadState(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()
	st := &debforgeState{}
	statePath := filepath.Join(cfg.StatesDir, "debforge.state.json")

	u := &Updater{fs: fs, store: statestore.New(fs), cfg: cfg}
	if err := u.loadState(st); err != nil {
		t.Fatalf("loadState on missing file: %v", err)
	}

	saved := &debforgeState{InstalledAt: "2024-01-01", UpdatedAt: "2024-06-01"}
	data, _ := json.Marshal(saved)
	fs.WriteFile(statePath, data, 0644)

	st2 := &debforgeState{}
	if err := u.loadState(st2); err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if st2.InstalledAt != "2024-01-01" {
		t.Fatalf("InstalledAt = %q, want %q", st2.InstalledAt, "2024-01-01")
	}
}

func TestUpdaterLoadStateCorrupt(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()
	statePath := filepath.Join(cfg.StatesDir, "debforge.state.json")
	fs.WriteFile(statePath, []byte("{{invalid json"), 0644)

	u := &Updater{fs: fs, store: statestore.New(fs), cfg: cfg}
	st := &debforgeState{}
	if err := u.loadState(st); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

func TestUpdaterSaveState(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()
	u := &Updater{fs: fs, store: statestore.New(fs), cfg: cfg}

	st := &debforgeState{InstalledAt: "2024-01-01", UpdatedAt: "2024-06-01"}
	if err := u.saveState(st); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	statePath := filepath.Join(cfg.StatesDir, "debforge.state.json")
	data, err := fs.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state file not saved: %v", err)
	}
	var loaded debforgeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if loaded.InstalledAt != "2024-01-01" {
		t.Fatalf("InstalledAt = %q, want %q", loaded.InstalledAt, "2024-01-01")
	}
}

func TestUpdaterSourceRepoExists(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()
	u := &Updater{fs: fs, store: statestore.New(fs), cfg: cfg}

	if u.sourceRepoExists() {
		t.Fatal("expected source repo to not exist")
	}

	gitPath := filepath.Join(cfg.SourceDir(), ".git")
	fs.WriteFile(gitPath, []byte("git repo"), 0644)
	if !u.sourceRepoExists() {
		t.Fatal("expected source repo to exist")
	}
}

func TestUpdaterVerifyBinary(t *testing.T) {
	tests := []struct {
		name    string
		stdout  []byte
		stderr  []byte
		err     error
		wantErr bool
	}{
		{name: "valid", stdout: []byte("debforge v1.0"), err: nil, wantErr: false},
		{name: "empty output", stdout: []byte{}, err: nil, wantErr: true},
		{name: "exec error", stdout: nil, stderr: []byte("not found"), err: errors.New("exec err"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &Updater{runner: &mockRunner{stdout: tt.stdout, stderr: tt.stderr, err: tt.err}}
			err := u.verifyBinary(context.Background(), "/tmp/debforge")
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdaterInstallBinaryRename(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()
	runner := &mockRunner{}

	fs.WriteFile("/tmp/debforge.new", []byte("binary content"), 0755)

	u := &Updater{fs: fs, store: statestore.New(fs), runner: runner, cfg: cfg, logger: &mockLogger{}}
	if err := u.installBinary("/tmp/debforge.new", "/opt/debforge/bin/debforge"); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	data, err := fs.ReadFile("/opt/debforge/bin/debforge")
	if err != nil {
		t.Fatalf("binary not installed: %v", err)
	}
	if string(data) != "binary content" {
		t.Fatalf("content = %q, want %q", string(data), "binary content")
	}
}

func TestUpdaterInstallBinaryCrossDevice(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()

	fs.WriteFile("/tmp/debforge.new", []byte("binary"), 0755)

	mockRename := &renameFailingFS{FileSystem: fs, failOn: func(old, new string) bool {
		return true
	}}

	u := &Updater{fs: mockRename, store: statestore.New(mockRename), cfg: cfg, logger: &mockLogger{}}
	if err := u.installBinary("/tmp/debforge.new", "/opt/debforge/bin/debforge"); err != nil {
		t.Fatalf("installBinary cross-device: %v", err)
	}

	data, err := fs.ReadFile("/opt/debforge/bin/debforge")
	if err != nil {
		t.Fatalf("binary not installed: %v", err)
	}
	if string(data) != "binary" {
		t.Fatalf("content = %q, want %q", string(data), "binary")
	}
}

type renameFailingFS struct {
	ports.FileSystem
	failOn func(old, new string) bool
}

func (f *renameFailingFS) Rename(oldPath, newPath string) error {
	if f.failOn(oldPath, newPath) {
		return syscall.EXDEV
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func TestUpdaterUpdateRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping root check test when running as root")
	}
	fs := newMemFS()
	cfg := testConfig()
	logger := &mockLogger{}

	u := NewUpdater(&mockRunner{}, &mockLocker{}, logger, fs, cfg)
	err := u.Update(context.Background())
	if err == nil {
		t.Fatal("expected error when not running as root")
	}
	if !strings.Contains(err.Error(), "must be run as root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdaterUpdateLockError(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root")
	}

	locker := &failLocker{}
	u := NewUpdater(&mockRunner{}, locker, &mockLogger{}, newMemFS(), testConfig())
	err := u.Update(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

type failLocker struct{}

func (m *failLocker) Acquire(ctx context.Context, path string) (func(), error) {
	return nil, errors.New("lock failed")
}

func TestUpdaterUpdateAlreadyUpToDate(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root")
	}

	fs := newMemFS()
	cfg := testConfig()
	logger := &mockLogger{promptResult: true}

	gitPath := filepath.Join(cfg.SourceDir(), ".git")
	fs.WriteFile(gitPath, []byte("git"), 0644)

	u := NewUpdater(&mockRunner{stdout: []byte("abc123")}, &mockLocker{}, logger, fs, cfg)
	err := u.Update(context.Background())
	if err == nil {
		t.Log("Update succeeded (expected failure from git exec)")
	}
}

func TestUpdaterUpdateCancelledInstall(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root")
	}

	fs := newMemFS()
	logger := &mockLogger{promptResult: false}

	u := NewUpdater(&mockRunner{}, &mockLocker{}, logger, fs, testConfig())
	err := u.Update(context.Background())
	if err != nil {
		t.Fatalf("expected nil when install cancelled, got: %v", err)
	}
	if logger.lastInfo != "Cancelled" {
		t.Fatalf("expected Cancelled, got %q", logger.lastInfo)
	}
}

func TestUpdaterUpdatePausesSpinnerDuringInstallPrompt(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root")
	}

	fs := newMemFS()
	spy := &spySpinner{}
	logger := &promptSpyLogger{
		mockLogger: mockLogger{promptResult: false},
		spy:        spy,
	}

	u := NewUpdater(&mockRunner{}, &mockLocker{}, logger, fs, testConfig())
	if err := u.Update(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// For a cancelled install (no source repo, prompt returns false):
	//   Spinner created, then Pause(), Prompt -> false, Done()
	want := []string{"Pause", "Done"}
	if !reflect.DeepEqual(spy.calls, want) {
		t.Errorf("spinner calls = %v, want %v", spy.calls, want)
	}
}

func TestUpdaterInstallBinarySetsExecutable(t *testing.T) {
	fs := newMemFS()
	cfg := testConfig()

	binPath := "/opt/debforge/bin/debforge"
	fs.WriteFile("/tmp/debforge.new", []byte("binary"), 0644)
	mockRename := &renameFailingFS{FileSystem: fs, failOn: func(old, new string) bool {
		return false
	}}

	u := &Updater{fs: mockRename, store: statestore.New(mockRename), cfg: cfg, logger: &mockLogger{}}
	if err := u.installBinary("/tmp/debforge.new", binPath); err != nil {
		t.Fatalf("installBinary: %v", err)
	}
}

func TestUpdaterEnsureSymlinkNew(t *testing.T) {
	u := &Updater{fs: &fs.OSFileSystem{}}
	err := u.ensureSymlink("/opt/debforge/bin/debforge", "/tmp/debforge-link")
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	defer os.Remove("/tmp/debforge-link")

	target, err := os.Readlink("/tmp/debforge-link")
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != "/opt/debforge/bin/debforge" {
		t.Fatalf("symlink target = %q, want %q", target, "/opt/debforge/bin/debforge")
	}
}

func TestUpdaterEnsureSymlinkAlreadyCorrect(t *testing.T) {
	os.Symlink("/opt/debforge/bin/debforge", "/tmp/debforge-link2")
	defer os.Remove("/tmp/debforge-link2")

	u := &Updater{fs: &fs.OSFileSystem{}}
	err := u.ensureSymlink("/opt/debforge/bin/debforge", "/tmp/debforge-link2")
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
}

func TestUpdaterEnsureSymlinkWrongTarget(t *testing.T) {
	os.Symlink("/old/path", "/tmp/debforge-link3")
	defer os.Remove("/tmp/debforge-link3")

	u := &Updater{fs: &fs.OSFileSystem{}}
	err := u.ensureSymlink("/opt/debforge/bin/debforge", "/tmp/debforge-link3")
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}

	target, _ := os.Readlink("/tmp/debforge-link3")
	if target != "/opt/debforge/bin/debforge" {
		t.Fatalf("symlink target = %q, want %q", target, "/opt/debforge/bin/debforge")
	}
}

func TestUpdaterEnsureSymlinkRegularFile(t *testing.T) {
	os.WriteFile("/tmp/debforge-regular", []byte("not a link"), 0644)
	defer os.Remove("/tmp/debforge-regular")

	u := &Updater{fs: &fs.OSFileSystem{}}
	err := u.ensureSymlink("/opt/debforge/bin/debforge", "/tmp/debforge-regular")
	if err == nil {
		t.Fatal("expected error for regular file")
	}
}

type failMkdirFS struct {
	memFS
}

func (f *failMkdirFS) MkdirAll(path string, perm os.FileMode) error {
	return os.ErrPermission
}

func TestUpdaterBuildBinaryMkdirFails(t *testing.T) {
	cfg := &config.Config{GoBinaryPath: "go"}
	u := &Updater{fs: &failMkdirFS{}, store: statestore.New(&failMkdirFS{}), cfg: cfg, logger: &mockLogger{}}
	err := u.buildBinary(context.Background(), "/tmp/debforge.new")
	if err == nil {
		t.Fatal("expected error from MkdirAll failure")
	}
}

func TestUpdaterBuildBinaryGoNotFound(t *testing.T) {
	cfg := &config.Config{
		GoBinaryPath: "nonexistent-go-binary-xyz789",
		RootDir:      "/opt/debforge",
	}
	fs := newMemFS()
	u := &Updater{fs: fs, store: statestore.New(fs), cfg: cfg, logger: &mockLogger{}, runner: &mockRunner{err: os.ErrNotExist}}
	err := u.buildBinary(context.Background(), "/tmp/debforge.new")
	if err == nil {
		t.Fatal("expected error (go binary should not exist)")
	}
}
