package deb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/domain/deployer"
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

type mockLogger struct{}

func (m *mockLogger) Info(format string, args ...any)               {}
func (m *mockLogger) Success(format string, args ...any)            {}
func (m *mockLogger) Warn(format string, args ...any)               {}
func (m *mockLogger) Error(format string, args ...any)              {}
func (m *mockLogger) Muted(format string, args ...any)              {}
func (m *mockLogger) Debug(format string, args ...any)              {}
func (m *mockLogger) Prompt(format string, args ...any) bool        { return true }
func (m *mockLogger) PromptInput(format string, args ...any) string { return "" }

type noopSpinner struct{}

func (noopSpinner) Done()          {}
func (noopSpinner) Fail()          {}
func (noopSpinner) Pause()         {}
func (noopSpinner) Resume()        {}
func (noopSpinner) SetDesc(string) {}

func (m *mockLogger) Spinner(ctx context.Context, desc string) ports.Spinner { return noopSpinner{} }
func (m *mockLogger) Progress(total int64, desc string) ports.Progress       { return nil }

type mockHTTP struct {
	resp       []byte
	err        error
	statusCode int
}

func (m *mockHTTP) Do(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	body := m.resp
	if body == nil {
		body = []byte{}
	}
	sc := m.statusCode
	if sc == 0 {
		sc = http.StatusOK
	}
	return &http.Response{Body: io.NopCloser(bytes.NewReader(body)), StatusCode: sc}, nil
}

type mockFS struct{}

func (m *mockFS) ReadFile(name string) ([]byte, error)                             { return nil, os.ErrNotExist }
func (m *mockFS) WriteFile(name string, data []byte, perm os.FileMode) error       { return nil }
func (m *mockFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error { return nil }
func (m *mockFS) ReadDir(name string) ([]os.DirEntry, error)                       { return nil, nil }
func (m *mockFS) Stat(name string) (os.FileInfo, error)                            { return nil, os.ErrNotExist }
func (m *mockFS) MkdirAll(path string, perm os.FileMode) error                     { return nil }
func (m *mockFS) RemoveAll(path string) error                                      { return nil }
func (m *mockFS) Chmod(name string, mode os.FileMode) error                        { return nil }
func (m *mockFS) Rename(oldPath, newPath string) error                             { return nil }
func (m *mockFS) Lstat(name string) (os.FileInfo, error)                           { return nil, os.ErrNotExist }
func (m *mockFS) Readlink(name string) (string, error)                             { return "", os.ErrNotExist }
func (m *mockFS) Symlink(target, link string) error                                { return nil }
func (m *mockFS) MkdirTemp(dir, pattern string) (string, error)                    { return "/tmp/test", nil }

func newTestInstaller() *Installer {
	return NewInstaller(
		&mockRunner{},
		&mockHTTP{},
		&mockLogger{},
		deployer.NewDeployer(&mockFS{}, &mockRunner{}, &mockLogger{}),
		&mockFS{},
	)
}

func TestInstallTypeMismatch(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Install(context.Background(), &pkg.Package{Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeApt}})
	if err == nil || !strings.Contains(err.Error(), "called for type") {
		t.Fatalf("expected type mismatch error, got %v", err)
	}
}

func TestInstallNoURL(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Install(context.Background(), &pkg.Package{Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeDeb}})
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("expected url required error, got %v", err)
	}
}

func TestRemoveMissingPackageField(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Remove(context.Background(), &pkg.Package{
		Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
	})
	if err == nil || !strings.Contains(err.Error(), "package is required") {
		t.Fatalf("expected 'package is required' error, got %v", err)
	}
}

func TestIsDebURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/pkg.deb", true},
		{"https://github.com/repo/releases/download/v1.0/pkg.deb", true},
		{"https://example.com/pkg.tar.gz", false},
		{"https://example.com/pkg", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isDebURL(tt.url)
		if got != tt.want {
			t.Errorf("isDebURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestStripDebRevision(t *testing.T) {
	tests := []struct {
		ver  string
		want string
	}{
		{"1.0-1", "1.0"},
		{"1.0", "1.0"},
		{"2.3.4-5ubuntu6", "2.3.4"},
		{"1:1.0-1", "1:1.0"},
		{"1.0-rc1-1", "1.0-rc1"}, // dash in upstream version
	}
	for _, tt := range tests {
		got := stripDebRevision(tt.ver)
		if got != tt.want {
			t.Errorf("stripDebRevision(%q) = %q, want %q", tt.ver, got, tt.want)
		}
	}
}

func TestInstalledDebVersion(t *testing.T) {
	runner := &mockRunner{stdout: []byte("1.0-1")}
	inst := NewInstaller(runner, &mockHTTP{}, &mockLogger{}, deployer.NewDeployer(&mockFS{}, runner, &mockLogger{}), &mockFS{})
	ver := inst.installedDebVersion(context.Background(), "testpkg")
	if ver != "1.0-1" {
		t.Fatalf("expected 1.0-1, got %q", ver)
	}
}

func TestInstalledDebVersionError(t *testing.T) {
	runner := &mockRunner{err: os.ErrNotExist}
	inst := NewInstaller(runner, &mockHTTP{}, &mockLogger{}, deployer.NewDeployer(&mockFS{}, runner, &mockLogger{}), &mockFS{})
	ver := inst.installedDebVersion(context.Background(), "testpkg")
	if ver != "" {
		t.Fatalf("expected empty version on error, got %q", ver)
	}
}

type stepResult struct {
	stdout []byte
	stderr []byte
	err    error
}

type stepRunner struct {
	results []stepResult
	pos     int
}

func (r *stepRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if r.pos >= len(r.results) {
		return nil, nil, nil
	}
	res := r.results[r.pos]
	r.pos++
	return res.stdout, res.stderr, res.err
}

func (r *stepRunner) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (r *stepRunner) RunWithSpinner(ctx context.Context, name string, args ...string) error {
	if r.pos >= len(r.results) {
		return nil
	}
	res := r.results[r.pos]
	r.pos++
	return res.err
}

func TestInstallReleaseNoMatchingAsset(t *testing.T) {
	http := &mockHTTP{resp: []byte(`{"tag_name":"v1.0","assets":[{"name":"other.tar.gz","browser_download_url":"https://example.com/other.tar.gz"}]}`)}
	inst := NewInstaller(&mockRunner{}, http, &mockLogger{}, deployer.NewDeployer(&mockFS{}, &mockRunner{}, &mockLogger{}), &mockFS{})
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		RepositorySpec: pkg.RepositorySpec{URL: "https://api.github.com/repos/owner/repo/releases/latest"},
		InstallSpec:    pkg.InstallSpec{Package: "testpkg"},
	})
	if err == nil || !strings.Contains(err.Error(), "no amd64 asset") {
		t.Fatalf("expected no matching asset error, got %v", err)
	}
}

func TestRemoveDpkgFails(t *testing.T) {
	runner := &mockRunner{err: os.ErrNotExist}
	inst := NewInstaller(runner, &mockHTTP{}, &mockLogger{}, deployer.NewDeployer(&mockFS{}, runner, &mockLogger{}), &mockFS{})
	err := inst.Remove(context.Background(), &pkg.Package{
		Metadata:    pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		InstallSpec: pkg.InstallSpec{Package: "testpkg"},
	})
	if err == nil || !strings.Contains(err.Error(), "purging") {
		t.Fatalf("expected purge error, got %v", err)
	}
}

func TestInstallDirectDebInstallFails(t *testing.T) {
	// stepRunner: dpkg-deb succeeds (version "2.0"), dpkg-query succeeds (version "1.0"),
	// then apt-get install fails
	runner := &stepRunner{results: []stepResult{
		{stdout: []byte("2.0")}, // dpkg-deb -f
		{stdout: []byte("1.0")}, // dpkg-query -W
		{err: os.ErrNotExist},   // apt-get install
	}}
	inst := NewInstaller(runner, &mockHTTP{}, &mockLogger{}, deployer.NewDeployer(&mockFS{}, runner, &mockLogger{}), &mockFS{})
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		RepositorySpec: pkg.RepositorySpec{URL: "https://example.com/pkg.deb"},
		InstallSpec:    pkg.InstallSpec{Package: "testpkg"},
	})
	if err == nil || !strings.Contains(err.Error(), "installing") {
		t.Fatalf("expected install error, got %v", err)
	}
}

func TestInstallDirectDebVersionMatch(t *testing.T) {
	runner := &stepRunner{results: []stepResult{
		{stdout: []byte("1.0-1")}, // dpkg-deb -f
		{stdout: []byte("1.0-1")}, // dpkg-query -W (exact match)
	}}
	inst := NewInstaller(runner, &mockHTTP{}, &mockLogger{}, deployer.NewDeployer(&mockFS{}, runner, &mockLogger{}), &mockFS{})
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		RepositorySpec: pkg.RepositorySpec{URL: "https://example.com/pkg.deb"},
		InstallSpec:    pkg.InstallSpec{Package: "testpkg"},
	})
	if err != nil {
		t.Fatalf("expected no error for up-to-date package, got %v", err)
	}
}

func TestInstallReleaseInfoTooLarge(t *testing.T) {
	old := releaseJSONLimit
	releaseJSONLimit = 100
	defer func() { releaseJSONLimit = old }()

	padded := strings.Repeat("a", 101)
	body := `{"tag_name":"` + padded + `"}`
	http := &mockHTTP{resp: []byte(body)}
	inst := NewInstaller(&mockRunner{}, http, &mockLogger{}, deployer.NewDeployer(&mockFS{}, &mockRunner{}, &mockLogger{}), &mockFS{})
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		RepositorySpec: pkg.RepositorySpec{URL: "https://api.github.com/repos/owner/repo/releases/latest"},
		InstallSpec:    pkg.InstallSpec{Package: "testpkg"},
	})
	if err == nil {
		t.Fatal("expected error for oversized release JSON")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit error, got: %v", err)
	}
}

func TestInstallReleaseNoTagName(t *testing.T) {
	http := &mockHTTP{resp: []byte(`{"tag_name":"","assets":[]}`)}
	inst := NewInstaller(&mockRunner{}, http, &mockLogger{}, deployer.NewDeployer(&mockFS{}, &mockRunner{}, &mockLogger{}), &mockFS{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := inst.Install(ctx, &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeDeb},
		RepositorySpec: pkg.RepositorySpec{URL: "https://api.github.com/repos/owner/repo/releases/latest"},
		InstallSpec:    pkg.InstallSpec{Package: "testpkg"},
	})
	if err == nil {
		t.Fatal("expected error for empty tag_name")
	}
}

func TestVerifySHA256(t *testing.T) {
	content := []byte("hello world")
	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	t.Run("match", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.deb")
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
		if err := verifySHA256(path, correctHash); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.deb")
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
		err := verifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Fatal("expected error for mismatched hash")
		}
		if !strings.Contains(err.Error(), "SHA-256 mismatch") {
			t.Fatalf("expected SHA-256 mismatch error, got: %v", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.deb")
		err := verifySHA256(path, correctHash)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "opening file") {
			t.Fatalf("expected opening file error, got: %v", err)
		}
	})
}
