package config

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

var testSys = &testutil.MockSystem{}

type mockFileSystem struct {
	files         map[string][]byte
	RemoveAllFunc func(path string) error
}

func newMockFS() *mockFileSystem {
	return &mockFileSystem{files: make(map[string][]byte)}
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, nil
	}
	return data, nil
}
func (m *mockFileSystem) WriteFile(path string, data []byte, perm int) error {
	m.files[path] = data
	return nil
}
func (m *mockFileSystem) Create(path string) (io.WriteCloser, error) { return nil, nil }
func (m *mockFileSystem) RemoveAll(path string) error {
	if m.RemoveAllFunc != nil {
		return m.RemoveAllFunc(path)
	}
	return nil
}
func (m *mockFileSystem) MkdirAll(path string, perm int) error     { return nil }
func (m *mockFileSystem) MkdirTemp(pattern string) (string, error) { return "", nil }
func (m *mockFileSystem) Stat(path string) (ports.FileInfo, error) { return nil, nil }
func (m *mockFileSystem) Glob(pattern string) ([]string, error)    { return nil, nil }
func (m *mockFileSystem) Walk(root string, fn func(path string, info ports.FileInfo, err error) error) error {
	return nil
}
func (m *mockFileSystem) Rename(oldPath, newPath string) error { return nil }
func (m *mockFileSystem) Symlink(target, link string) error    { return nil }
func (m *mockFileSystem) Readlink(path string) (string, error) { return "", nil }
func (m *mockFileSystem) Exists(path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}
func (m *mockFileSystem) Chown(path string, uid, gid int) error { return nil }

var _ ports.Spinner = (*testutil.MockSpinner)(nil)

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func TestComputeConfigHash_deterministic(t *testing.T) {
	p := &pkg.Package{
		Configs: map[string]string{
			"/b.conf": "bb",
			"/a.conf": "aa",
		},
		UserConfigs: map[string]string{
			"~/z.conf": "zz",
			"~/y.conf": "yy",
		},
	}
	h1 := computeConfigHash(p)
	h2 := computeConfigHash(p)
	if h1 != h2 {
		t.Errorf("expected deterministic hash, got %q vs %q", h1, h2)
	}

	p2 := &pkg.Package{
		Configs: map[string]string{
			"/a.conf": "aa",
			"/b.conf": "bb",
		},
		UserConfigs: map[string]string{
			"~/y.conf": "yy",
			"~/z.conf": "zz",
		},
	}
	h3 := computeConfigHash(p2)
	if h1 != h3 {
		t.Errorf("expected hash independent of map order, got %q vs %q", h1, h3)
	}
}

func TestComputeConfigHash_empty(t *testing.T) {
	h := computeConfigHash(&pkg.Package{})
	if h == "" {
		t.Error("expected non-empty hash even for empty config")
	}
}

func TestComputeConfigHash_differsFromRegularConfig(t *testing.T) {
	p1 := &pkg.Package{
		UserConfigs: map[string]string{"~/.config/foo": "user data"},
	}
	p2 := &pkg.Package{
		Configs: map[string]string{"/etc/foo": "system data"},
	}
	h1 := computeConfigHash(p1)
	h2 := computeConfigHash(p2)
	if h1 == h2 {
		t.Error("expected different hash for user configs vs regular configs")
	}
}
