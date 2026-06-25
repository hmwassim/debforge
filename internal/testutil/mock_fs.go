package testutil

import (
	"fmt"
	"io"

	"github.com/hmwassim/debforge/internal/ports"
)

// MockFileSystem is a minimal in-memory ports.FileSystem for unit tests.
// It only implements the bookkeeping installers actually rely on (file
// contents + existence); directory semantics are intentionally trivial
// (MkdirAll/MkdirTemp/RemoveAll are no-ops/always-succeed) since no
// current caller needs real directory listing behavior from a mock.
type MockFileSystem struct {
	Files map[string][]byte

	// TempDir is returned by MkdirTemp. Defaults to "/tmp/debforge-test"
	// when empty, so callers don't need to set it unless a test cares
	// about the exact path.
	TempDir string
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

func (m *MockFileSystem) WriteFile(path string, data []byte, _ int) error {
	m.Files[path] = data
	return nil
}

func (m *MockFileSystem) Create(_ string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("MockFileSystem.Create not implemented")
}

func (m *MockFileSystem) RemoveAll(path string) error {
	delete(m.Files, path)
	return nil
}

func (m *MockFileSystem) MkdirAll(_ string, _ int) error { return nil }
func (m *MockFileSystem) MkdirTemp(_ string) (string, error) {
	if m.TempDir != "" {
		return m.TempDir, nil
	}
	return "/tmp/debforge-test", nil
}

func (m *MockFileSystem) Stat(_ string) (ports.FileInfo, error) {
	return nil, fmt.Errorf("MockFileSystem.Stat not implemented")
}

func (m *MockFileSystem) Glob(_ string) ([]string, error)       { return nil, nil }
func (m *MockFileSystem) Walk(_ string, _ func(string, ports.FileInfo, error) error) error {
	return nil
}
func (m *MockFileSystem) Rename(oldPath, newPath string) error {
	if data, ok := m.Files[oldPath]; ok {
		m.Files[newPath] = data
		delete(m.Files, oldPath)
	}
	return nil
}
func (m *MockFileSystem) Symlink(_, _ string) error         { return nil }
func (m *MockFileSystem) Readlink(_ string) (string, error) { return "", nil }
func (m *MockFileSystem) Exists(path string) (bool, error) {
	_, ok := m.Files[path]
	return ok, nil
}

var _ ports.FileSystem = (*MockFileSystem)(nil)
