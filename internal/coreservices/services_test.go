package services

import (
	"context"
	"os"
	"time"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
)

type memFileInfo struct {
	name string
	mode os.FileMode
}

func (m *memFileInfo) Name() string       { return m.name }
func (m *memFileInfo) Size() int64        { return 0 }
func (m *memFileInfo) Mode() os.FileMode  { return m.mode }
func (m *memFileInfo) ModTime() time.Time { return time.Time{} }
func (m *memFileInfo) IsDir() bool        { return false }
func (m *memFileInfo) Sys() any           { return nil }

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

type mockSpinner struct{}

func (m *mockSpinner) Done()              {}
func (m *mockSpinner) Fail()              {}
func (m *mockSpinner) Pause()             {}
func (m *mockSpinner) Resume()            {}
func (m *mockSpinner) SetDesc(string)     {}
func (m *mockSpinner) DoneWarn()          {}

type mockUI struct{}

func (m *mockUI) Info(format string, args ...any)                        {}
func (m *mockUI) Success(format string, args ...any)                     {}
func (m *mockUI) Warn(format string, args ...any)                        {}
func (m *mockUI) Error(format string, args ...any)                       {}
func (m *mockUI) Muted(format string, args ...any)                       {}
func (m *mockUI) Debug(format string, args ...any)                       {}
func (m *mockUI) Prompt(format string, args ...any) bool                 { return true }
func (m *mockUI) PromptInput(format string, args ...any) string          { return "" }
func (m *mockUI) Spinner(ctx context.Context, desc string) ports.Spinner { return &mockSpinner{} }
func (m *mockUI) Progress(total int64, desc string) ports.Progress       { return nil }

type memFS struct {
	files    map[string][]byte
	modes    map[string]os.FileMode
	symlinks map[string]string
}

func newMemFS() *memFS {
	return &memFS{
		files:    map[string][]byte{},
		modes:    map[string]os.FileMode{},
		symlinks: map[string]string{},
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
	f.files[name] = data
	f.modes[name] = perm
	return nil
}

func (f *memFS) AtomicWriteFile(name string, data []byte, perm os.FileMode) error {
	f.files[name] = data
	f.modes[name] = perm
	return nil
}

func (f *memFS) ReadDir(name string) ([]os.DirEntry, error) {
	return nil, nil
}

func (f *memFS) Stat(name string) (os.FileInfo, error) {
	mode, ok := f.modes[name]
	if !ok {
		if _, ok := f.files[name]; !ok {
			return nil, os.ErrNotExist
		}
		mode = 0644
	}
	return &memFileInfo{name: name, mode: mode}, nil
}

func (f *memFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (f *memFS) RemoveAll(path string) error {
	delete(f.files, path)
	delete(f.modes, path)
	delete(f.symlinks, path)
	return nil
}

func (f *memFS) Chmod(name string, mode os.FileMode) error {
	f.modes[name] = mode
	return nil
}

func (f *memFS) Rename(oldPath, newPath string) error {
	data, ok := f.files[oldPath]
	if !ok {
		return os.ErrNotExist
	}
	f.files[newPath] = data
	f.modes[newPath] = f.modes[oldPath]
	delete(f.files, oldPath)
	delete(f.modes, oldPath)
	return nil
}

func (f *memFS) Lstat(name string) (os.FileInfo, error) {
	if _, ok := f.symlinks[name]; ok {
		return &memFileInfo{name: name, mode: os.ModeSymlink | 0644}, nil
	}
	return f.Stat(name)
}

func (f *memFS) Readlink(name string) (string, error) {
	target, ok := f.symlinks[name]
	if !ok {
		return "", os.ErrNotExist
	}
	return target, nil
}

func (f *memFS) Symlink(target, link string) error {
	f.symlinks[link] = target
	f.modes[link] = os.ModeSymlink | 0644
	return nil
}

func (f *memFS) MkdirTemp(dir, pattern string) (string, error) {
	return "/tmp/test", nil
}

type mockLocker struct{}

func (m *mockLocker) Acquire(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}

type mockInstaller struct {
	installErr error
	removeErr  error
	updateErr  error
	removed    []string
}

func (m *mockInstaller) Install(ctx context.Context, pkg *pkg.Package, _ ports.Spinner) error {
	return m.installErr
}

func (m *mockInstaller) Remove(ctx context.Context, pkg *pkg.Package, _ ports.Spinner) error {
	m.removed = append(m.removed, pkg.Name)
	return m.removeErr
}

func (m *mockInstaller) Update(ctx context.Context, pkg *pkg.Package, _ ports.Spinner) error {
	return m.updateErr
}
