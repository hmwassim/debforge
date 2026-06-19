package source

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

func (noopSpinner) Done()   {}
func (noopSpinner) Fail()   {}
func (noopSpinner) Pause()  {}
func (noopSpinner) Resume() {}

func (m *mockLogger) Spinner(ctx context.Context, desc string) ports.Spinner { return noopSpinner{} }
func (m *mockLogger) Progress(total int64, desc string) ports.Progress       { return nil }

type memFS struct {
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}}
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
	if _, ok := f.files[name]; !ok {
		return nil, os.ErrNotExist
	}
	return nil, nil
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

func newTestInstaller() *Installer {
	return NewInstaller(&mockRunner{}, &mockLogger{}, newMemFS())
}

func TestTypeMismatch(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Install(context.Background(), &pkg.Package{Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeDeb}})
	if err == nil || !strings.Contains(err.Error(), "called for type") {
		t.Fatalf("expected type mismatch error, got %v", err)
	}
}

func TestRemoveTypeMismatch(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Remove(context.Background(), &pkg.Package{Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeDeb}})
	if err == nil || !strings.Contains(err.Error(), "called for type") {
		t.Fatalf("expected type mismatch error, got %v", err)
	}
}

func TestRemoveNoPostRemove(t *testing.T) {
	inst := newTestInstaller()
	err := inst.Remove(context.Background(), &pkg.Package{Metadata: pkg.Metadata{Name: "test", Type: pkg.TypeSource}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRemoveWithPostRemove(t *testing.T) {
	runner := &mockRunner{}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Remove(context.Background(), &pkg.Package{
		Metadata:    pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		InstallSpec: pkg.InstallSpec{PostRemove: "echo removed"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestMissingGit(t *testing.T) {
	runner := &mockRunner{err: os.ErrNotExist}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
	})
	if err == nil {
		t.Fatal("expected error for missing git")
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

func TestGitCloneFails(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: nil},            // which git
			{err: os.ErrNotExist}, // git clone (all attempts fail, ctx timeout cuts retries)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(ctx, &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
	})
	if err == nil || !strings.Contains(err.Error(), "cloning") {
		t.Fatalf("expected cloning error, got %v", err)
	}
}

func TestInstallShNotFound(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: nil},                    // which git
			{err: nil},                    // git clone
			{stdout: []byte("abc123def")}, // git rev-parse HEAD
		},
	}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
	})
	if err == nil || !strings.Contains(err.Error(), "install.sh not found") {
		t.Fatalf("expected install.sh not found error, got %v", err)
	}
}

func TestInstallShFails(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: nil},                    // which git
			{err: nil},                    // git clone
			{stdout: []byte("abc123def")}, // git rev-parse HEAD
			{err: os.ErrNotExist},         // install.sh execution
		},
	}
	fs := newMemFS()
	fs.WriteFile("/tmp/test/install.sh", []byte("#!/bin/sh\necho ok"), 0755)
	inst := NewInstaller(runner, &mockLogger{}, fs)
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
	})
	if err == nil || !strings.Contains(err.Error(), "install.sh") {
		t.Fatalf("expected install.sh error, got %v", err)
	}
}

func TestInstallSuccess(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: nil},                    // which git
			{err: nil},                    // git clone
			{stdout: []byte("abc123def")}, // git rev-parse HEAD
			{err: nil},                    // install.sh
		},
	}
	fs := newMemFS()
	fs.WriteFile("/tmp/test/install.sh", []byte("#!/bin/sh\necho ok"), 0755)
	inst := NewInstaller(runner, &mockLogger{}, fs)
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestInstallSkipCloneWithPostInstall(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: nil}, // sh -c PostInstall
		},
	}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:    pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		InstallSpec: pkg.InstallSpec{SkipClone: true, PostInstall: "echo done"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestInstallAlreadyUpToDate(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{stdout: []byte("1.2.3")}, // sh -c VersionCmd
		},
	}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:    pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		InstallSpec: pkg.InstallSpec{SkipClone: true, VersionCmd: "echo 1.2.3", Version: "1.2.3"},
	})
	if err != nil {
		t.Fatalf("expected no error (already up to date), got %v", err)
	}
}

func TestInstallForceInstallBypassesVersionCheck(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{stdout: []byte("1.2.3")}, // sh -c VersionCmd (for display, not skip)
			{err: nil},                // sh -c PostInstall
		},
	}
	inst := NewInstaller(runner, &mockLogger{}, newMemFS())
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:    pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		InstallSpec: pkg.InstallSpec{SkipClone: true, VersionCmd: "echo 1.2.3", Version: "1.2.3", ForceInstall: true},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestInstallChecksInstallsDependency(t *testing.T) {
	runner := &stepRunner{
		results: []stepResult{
			{err: os.ErrNotExist},         // which build-essential (not found)
			{err: nil},                    // apt-get install build-essential
			{err: nil},                    // which git
			{err: nil},                    // git clone
			{stdout: []byte("abc123def")}, // git rev-parse HEAD
			{err: nil},                    // install.sh
		},
	}
	fs := newMemFS()
	fs.WriteFile("/tmp/test/install.sh", []byte("#!/bin/sh\necho ok"), 0755)
	inst := NewInstaller(runner, &mockLogger{}, fs)
	err := inst.Install(context.Background(), &pkg.Package{
		Metadata:       pkg.Metadata{Name: "test", Type: pkg.TypeSource},
		RepositorySpec: pkg.RepositorySpec{Repo: "https://example.com/repo.git"},
		InstallSpec:    pkg.InstallSpec{Checks: []string{"build-essential"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
