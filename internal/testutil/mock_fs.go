package testutil

import (
	"bytes"
	"fmt"
	"io"

	"github.com/hmwassim/debforge/internal/ports"
)

// MockFileSystem is a minimal in-memory ports.FileSystem for unit tests.
// It only implements the bookkeeping installers actually rely on (file
// contents + existence); directory semantics are intentionally trivial
// (MkdirAll/MkdirTemp/RemoveAll are no-ops/always-succeed) since no
// current caller needs real directory listing behavior from a mock.
//
// Each method has a corresponding Func field (e.g. MkdirAllFunc) that, when
// set, is called instead of the default implementation. This lets tests
// inject errors or custom behavior without creating wrapper types.
type MockFileSystem struct {
	Files map[string][]byte

	// TempDir is returned by MkdirTemp. Defaults to "/tmp/debforge-test"
	// when empty, so callers don't need to set it unless a test cares
	// about the exact path.
	TempDir string

	CreateFunc    func(path string) (io.WriteCloser, error)
	MkdirAllFunc  func(path string, perm int) error
	RemoveAllFunc func(path string) error
	RenameFunc    func(oldPath, newPath string) error
	SymlinkFunc   func(target, link string) error
	ReadlinkFunc  func(path string) (string, error)
	ExistsFunc    func(path string) (bool, error)
	StatFunc      func(path string) (ports.FileInfo, error)
	WalkFunc      func(root string, fn func(path string, info ports.FileInfo, err error) error) error
	WriteFileFunc func(path string, data []byte, perm int) error
	ChownFunc     func(path string, uid, gid int) error
}

// NewMockFileSystem returns an empty MockFileSystem ready to use.
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{Files: make(map[string][]byte)}
}

func (m *MockFileSystem) ReadFile(path string) ([]byte, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, fmt.Errorf("mock fs: %s: no such file", path)
	}
	return data, nil
}

func (m *MockFileSystem) WriteFile(path string, data []byte, perm int) error {
	if m.WriteFileFunc != nil {
		return m.WriteFileFunc(path, data, perm)
	}
	m.Files[path] = data
	return nil
}

func (m *MockFileSystem) Create(path string) (io.WriteCloser, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(path)
	}
	return &memWriteCloser{fs: m, path: path}, nil
}

// memWriteCloser is an io.WriteCloser that buffers writes and writes to the
// Files map on Close.
type memWriteCloser struct {
	fs   *MockFileSystem
	path string
	buf  bytes.Buffer
}

func (w *memWriteCloser) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *memWriteCloser) Close() error {
	w.fs.Files[w.path] = w.buf.Bytes()
	return nil
}

func (m *MockFileSystem) RemoveAll(path string) error {
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(path)
	}
	delete(m.Files, path)
	return nil
}

func (m *MockFileSystem) MkdirAll(path string, perm int) error {
	if m.MkdirAllFunc != nil {
		return m.MkdirAllFunc(path, perm)
	}
	return nil
}
func (m *MockFileSystem) MkdirTemp(_ string) (string, error) {
	if m.TempDir != "" {
		return m.TempDir, nil
	}
	return "/tmp/debforge-test", nil
}

func (m *MockFileSystem) Stat(path string) (ports.FileInfo, error) {
	if m.StatFunc != nil {
		return m.StatFunc(path)
	}
	return nil, fmt.Errorf("MockFileSystem.Stat not implemented")
}

func (m *MockFileSystem) Glob(_ string) ([]string, error) { return nil, nil }
func (m *MockFileSystem) Walk(root string, fn func(string, ports.FileInfo, error) error) error {
	if m.WalkFunc != nil {
		return m.WalkFunc(root, fn)
	}
	return nil
}
func (m *MockFileSystem) Rename(oldPath, newPath string) error {
	if m.RenameFunc != nil {
		return m.RenameFunc(oldPath, newPath)
	}
	if data, ok := m.Files[oldPath]; ok {
		m.Files[newPath] = data
		delete(m.Files, oldPath)
	}
	return nil
}
func (m *MockFileSystem) Symlink(target, link string) error {
	if m.SymlinkFunc != nil {
		return m.SymlinkFunc(target, link)
	}
	return nil
}
func (m *MockFileSystem) Readlink(path string) (string, error) {
	if m.ReadlinkFunc != nil {
		return m.ReadlinkFunc(path)
	}
	return "", nil
}
func (m *MockFileSystem) Exists(path string) (bool, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(path)
	}
	_, ok := m.Files[path]
	return ok, nil
}

func (m *MockFileSystem) Chown(path string, uid, gid int) error {
	if m.ChownFunc != nil {
		return m.ChownFunc(path, uid, gid)
	}
	return nil
}

var _ ports.FileSystem = (*MockFileSystem)(nil)
