package deployer

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

type memFS struct {
	files map[string][]byte
	stats map[string]os.FileInfo
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, stats: map[string]os.FileInfo{}}
}

func (f *memFS) ReadFile(name string) ([]byte, error) {
	data, ok := f.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *memFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	f.files[name] = data
	return nil
}

func (f *memFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error {
	f.files[name] = data
	return nil
}

func (f *memFS) ReadDir(name string) ([]os.DirEntry, error) {
	return nil, nil
}

func (f *memFS) Stat(name string) (os.FileInfo, error) {
	fi, ok := f.stats[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return fi, nil
}

func (f *memFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (f *memFS) RemoveAll(path string) error {
	delete(f.files, path)
	return nil
}

func (f *memFS) Chmod(name string, mode os.FileMode) error {
	return nil
}

func (f *memFS) Rename(oldPath, newPath string) error {
	return nil
}

func (f *memFS) MkdirTemp(dir, pattern string) (string, error) {
	return "/tmp/test", nil
}

func (f *memFS) Lstat(name string) (os.FileInfo, error) {
	return os.Stat(name)
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

type spyLogger struct {
	warns []string
}

func (l *spyLogger) Info(format string, args ...any)                        {}
func (l *spyLogger) Success(format string, args ...any)                     {}
func (l *spyLogger) Warn(format string, args ...any)                        { l.warns = append(l.warns, format) }
func (l *spyLogger) Error(format string, args ...any)                       {}
func (l *spyLogger) Muted(format string, args ...any)                       {}
func (l *spyLogger) Debug(format string, args ...any)                       {}
func (l *spyLogger) Prompt(format string, args ...any) bool                 { return true }
func (l *spyLogger) PromptInput(format string, args ...any) string          { return "" }
func (l *spyLogger) Spinner(ctx context.Context, desc string) ports.Spinner { return nil }
func (l *spyLogger) Progress(total int64, desc string) ports.Progress       { return nil }

func TestDeployNewFile(t *testing.T) {
	fs := newMemFS()
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.Deploy(ctx, "hello world", "/etc/test/hello.conf", 0644); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	data, ok := fs.files["/etc/test/hello.conf"]
	if !ok {
		t.Fatal("expected file to be written")
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

func TestDeploySkipOnMatch(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/test/hello.conf"] = []byte("hello world")
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.Deploy(ctx, "hello world", "/etc/test/hello.conf", 0644); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
}

func TestDeployUpdateOnMismatch(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/test/hello.conf"] = []byte("old content")
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.Deploy(ctx, "new content", "/etc/test/hello.conf", 0644); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if string(fs.files["/etc/test/hello.conf"]) != "new content" {
		t.Fatalf("expected 'new content', got %q", string(fs.files["/etc/test/hello.conf"]))
	}
}

func TestDeployUserConfig(t *testing.T) {
	fs := newMemFS()
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.DeployUserConfig(ctx, "test data", ".config/test", "testuser"); err != nil {
		t.Fatalf("DeployUserConfig failed: %v", err)
	}
}

func TestDeployUserConfigPathTraversal(t *testing.T) {
	fs := newMemFS()
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	err := d.DeployUserConfig(ctx, "data", "../../etc/passwd", "testuser")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected 'escapes' error, got: %v", err)
	}
}

func TestRemoveUserConfigsPathTraversal(t *testing.T) {
	fs := newMemFS()
	logger := &spyLogger{}
	d := NewDeployer(fs, &mockRunner{}, logger)
	ctx := context.Background()

	err := d.RemoveUserConfigs(ctx, map[string]string{"../../etc/passwd": ""}, "testuser")
	if err != nil {
		t.Fatalf("RemoveUserConfigs failed: %v", err)
	}
	if len(logger.warns) == 0 {
		t.Fatal("expected warning for path traversal")
	}
}

func TestRunPostInstall(t *testing.T) {
	runner := &mockRunner{stdout: []byte("ok")}
	d := NewDeployer(newMemFS(), runner, &spyLogger{})
	ctx := context.Background()

	if err := d.RunPostInstall(ctx, "echo done"); err != nil {
		t.Fatalf("RunPostInstall failed: %v", err)
	}
}

func TestRunPostRemove(t *testing.T) {
	d := NewDeployer(newMemFS(), &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.RunPostRemove(ctx, "echo done"); err != nil {
		t.Fatalf("RunPostRemove failed: %v", err)
	}
}

func TestUserHomeDir(t *testing.T) {
	if got := UserHomeDir("root"); got != "/root" {
		t.Fatalf("expected /root, got %s", got)
	}
	dir := UserHomeDir("nonexistent_user_xyz")
	if dir == "" {
		t.Fatal("expected non-empty home dir")
	}
}

func TestDeployPackageConfigs(t *testing.T) {
	fs := newMemFS()
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.DeployPackageConfigs(ctx, map[string]string{
		"/etc/test/app.conf": "app config",
	}, nil); err != nil {
		t.Fatalf("DeployPackageConfigs failed: %v", err)
	}
	if string(fs.files["/etc/test/app.conf"]) != "app config" {
		t.Fatal("expected app config to be written")
	}
}

func TestRemoveConfigs(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/test/app.conf"] = []byte("data")
	d := NewDeployer(fs, &mockRunner{}, &spyLogger{})
	ctx := context.Background()

	if err := d.RemoveConfigs(ctx, map[string]string{"/etc/test/app.conf": ""}); err != nil {
		t.Fatalf("RemoveConfigs failed: %v", err)
	}
	if _, ok := fs.files["/etc/test/app.conf"]; ok {
		t.Fatal("expected file to be removed")
	}
}
