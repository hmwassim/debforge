package installers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
)

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

type memFS struct {
	files map[string][]byte
	stats map[string]int64
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, stats: map[string]int64{}}
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
	size, ok := f.stats[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &memFileInfo{name: name, size: size}, nil
}

func (f *memFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (f *memFS) RemoveAll(path string) error {
	delete(f.files, path)
	delete(f.stats, path)
	return nil
}

func (f *memFS) Chmod(name string, mode os.FileMode) error {
	return nil
}

func (f *memFS) Rename(oldPath, newPath string) error {
	f.files[newPath] = f.files[oldPath]
	delete(f.files, oldPath)
	return nil
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

func (f *memFS) MkdirTemp(dir, pattern string) (string, error) {
	return "/tmp/test", nil
}

type memFileInfo struct {
	name string
	size int64
}

func (m *memFileInfo) Name() string       { return m.name }
func (m *memFileInfo) Size() int64        { return m.size }
func (m *memFileInfo) Mode() os.FileMode  { return 0644 }
func (m *memFileInfo) ModTime() time.Time { return time.Time{} }
func (m *memFileInfo) IsDir() bool        { return false }
func (m *memFileInfo) Sys() any           { return nil }

type mockUI struct{}

func (m *mockUI) Info(format string, args ...any)                        {}
func (m *mockUI) Success(format string, args ...any)                     {}
func (m *mockUI) Warn(format string, args ...any)                        {}
func (m *mockUI) Error(format string, args ...any)                       {}
func (m *mockUI) Muted(format string, args ...any)                       {}
func (m *mockUI) Debug(format string, args ...any)                       {}
func (m *mockUI) Prompt(format string, args ...any) bool                 { return true }
func (m *mockUI) PromptInput(format string, args ...any) string          { return "" }
func (m *mockUI) Spinner(ctx context.Context, desc string) ports.Spinner { return nil }
func (m *mockUI) Progress(total int64, desc string) ports.Progress       { return nil }

type mockHTTP struct {
	respBody []byte
	status   int
}

func (m *mockHTTP) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(m.respBody)),
		StatusCode: m.status,
	}, nil
}

func newTestRepoManager(fs ports.FileSystem, runner ports.CommandRunner, http ports.HTTPClient) *RepoManager {
	return NewRepoManager(apt.NewService(runner, &mockUI{}), runner, fs, http, &mockUI{})
}

func TestEnsureRepoNoop(t *testing.T) {
	m := newTestRepoManager(newMemFS(), &mockRunner{}, &mockHTTP{})
	ctx := context.Background()

	err := m.EnsureRepo(ctx, &pkg.Package{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureRepoWithKey(t *testing.T) {
	fs := newMemFS()
	http := &mockHTTP{respBody: []byte("key data"), status: 200}
	m := newTestRepoManager(fs, &mockRunner{}, http)
	ctx := context.Background()

	err := m.EnsureRepo(ctx, &pkg.Package{
		Metadata: pkg.Metadata{Name: "test"},
		RepositorySpec: pkg.RepositorySpec{
			KeyURL:     "https://example.com/key.gpg",
			KeyPath:    "/etc/apt/keyrings/test.gpg",
			KeyDearmor: false,
			SourcePath: "/etc/apt/sources.list.d/test.sources",
			Sources:    "deb https://example.com bookworm main",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(fs.files["/etc/apt/keyrings/test.gpg"]) != "key data" {
		t.Fatal("expected key to be downloaded")
	}
	if string(fs.files["/etc/apt/sources.list.d/test.sources"]) != "deb https://example.com bookworm main" {
		t.Fatal("expected sources to be written")
	}
}

func TestEnsureRepoKeyCached(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/apt/keyrings/test.gpg"] = []byte("existing key")
	fs.stats["/etc/apt/keyrings/test.gpg"] = 12
	fs.files["/etc/apt/sources.list.d/test.sources"] = []byte("deb https://example.com bookworm main")

	m := newTestRepoManager(fs, &mockRunner{}, &mockHTTP{})
	ctx := context.Background()

	err := m.EnsureRepo(ctx, &pkg.Package{
		Metadata: pkg.Metadata{Name: "test"},
		RepositorySpec: pkg.RepositorySpec{
			KeyURL:     "https://example.com/key.gpg",
			KeyPath:    "/etc/apt/keyrings/test.gpg",
			KeyDearmor: false,
			SourcePath: "/etc/apt/sources.list.d/test.sources",
			Sources:    "deb https://example.com bookworm main",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanupRepoExtrepo(t *testing.T) {
	runner := &mockRunner{}
	m := newTestRepoManager(newMemFS(), runner, &mockHTTP{})
	ctx := context.Background()

	m.CleanupRepo(ctx, &pkg.Package{
		Metadata: pkg.Metadata{Name: "test"},
		RepositorySpec: pkg.RepositorySpec{
			Extrepo: "extrepo-test",
		},
	})
}

func TestCleanupRepoSourceAndKey(t *testing.T) {
	fs := newMemFS()
	fs.files["/etc/apt/sources.list.d/test.sources"] = []byte("deb ...")
	fs.files["/etc/apt/keyrings/test.gpg"] = []byte("key")

	m := newTestRepoManager(fs, &mockRunner{}, &mockHTTP{})
	ctx := context.Background()

	m.CleanupRepo(ctx, &pkg.Package{
		Metadata: pkg.Metadata{Name: "test"},
		RepositorySpec: pkg.RepositorySpec{
			SourcePath: "/etc/apt/sources.list.d/test.sources",
			KeyPath:    "/etc/apt/keyrings/test.gpg",
		},
	})

	if _, ok := fs.files["/etc/apt/sources.list.d/test.sources"]; ok {
		t.Fatal("expected source file to be removed")
	}
	if _, ok := fs.files["/etc/apt/keyrings/test.gpg"]; ok {
		t.Fatal("expected key file to be removed")
	}
}
