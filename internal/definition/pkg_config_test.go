package definition

import (
	"errors"
	"io"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

type memFile struct {
	content string
}

type memFS struct {
	files map[string]*memFile
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	f, ok := m.files[path]
	if !ok {
		return nil, errors.New("file not found")
	}
	return []byte(f.content), nil
}

func (m *memFS) WriteFile(path string, data []byte, perm int) error {
	m.files[path] = &memFile{content: string(data)}
	return nil
}

func (m *memFS) Exists(path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

func (m *memFS) RemoveAll(path string) error { return nil }
func (m *memFS) MkdirAll(path string, perm int) error {
	return nil
}
func (m *memFS) MkdirTemp(pattern string) (string, error) {
	return "/tmp", nil
}
func (m *memFS) Create(path string) (io.WriteCloser, error) {
	return nil, nil
}
func (m *memFS) Stat(path string) (ports.FileInfo, error) {
	return nil, nil
}
func (m *memFS) Glob(pattern string) ([]string, error) {
	return nil, nil
}
func (m *memFS) Rename(oldPath, newPath string) error {
	return nil
}
func (m *memFS) Symlink(target, link string) error { return nil }
func (m *memFS) Readlink(path string) (string, error) {
	return "", nil
}
func (m *memFS) Walk(root string, fn func(path string, info ports.FileInfo, err error) error) error {
	return nil
}

func TestContainsNewline(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"hello", false},
		{"hello\nworld", true},
		{"\n", true},
		{"", false},
		{"line1\nline2\nline3", true},
	}
	for _, tc := range tests {
		got := containsNewline(tc.s)
		if got != tc.want {
			t.Errorf("containsNewline(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestResolveConfigFiles_nil(t *testing.T) {
	fs := &memFS{}
	got, err := resolveConfigFiles(nil, fs, "/configs")
	if err != nil {
		t.Fatalf("resolveConfigFiles(nil) = %v", err)
	}
	if got != nil {
		t.Errorf("resolveConfigFiles(nil) = %v, want nil", got)
	}
}

func TestResolveConfigFiles_inline(t *testing.T) {
	fs := &memFS{}
	raw := map[string]string{
		"/etc/foo.conf": "inline content\nline2",
	}
	got, err := resolveConfigFiles(raw, fs, "/configs")
	if err != nil {
		t.Fatalf("resolveConfigFiles: %v", err)
	}
	if got["/etc/foo.conf"] != "inline content\nline2" {
		t.Errorf("got[foo] = %q, want inline content", got["/etc/foo.conf"])
	}
}

func TestResolveConfigFiles_fileRef(t *testing.T) {
	fs := &memFS{
		files: map[string]*memFile{
			"/configs/foo.conf": {content: "file content"},
		},
	}
	raw := map[string]string{
		"/etc/foo.conf": "foo.conf",
	}
	got, err := resolveConfigFiles(raw, fs, "/configs")
	if err != nil {
		t.Fatalf("resolveConfigFiles: %v", err)
	}
	if got["/etc/foo.conf"] != "file content" {
		t.Errorf("got[foo] = %q, want %q", got["/etc/foo.conf"], "file content")
	}
}

func TestResolveConfigFiles_missingFile(t *testing.T) {
	fs := &memFS{}
	raw := map[string]string{
		"/etc/missing.conf": "does-not-exist",
	}
	_, err := resolveConfigFiles(raw, fs, "/configs")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestResolveConfigFiles_emptyValue(t *testing.T) {
	fs := &memFS{}
	raw := map[string]string{
		"/etc/foo.conf": "",
	}
	got, err := resolveConfigFiles(raw, fs, "/configs")
	if err != nil {
		t.Fatalf("resolveConfigFiles: %v", err)
	}
	if got["/etc/foo.conf"] != "" {
		t.Errorf("got[foo] = %q, want empty", got["/etc/foo.conf"])
	}
}

func TestResolveConfigFiles_mixed(t *testing.T) {
	fs := &memFS{
		files: map[string]*memFile{
			"/configs/bar.conf": {content: "bar content"},
		},
	}
	raw := map[string]string{
		"/etc/foo.conf": "inline\ncontent",
		"/etc/bar.conf": "bar.conf",
	}
	got, err := resolveConfigFiles(raw, fs, "/configs")
	if err != nil {
		t.Fatalf("resolveConfigFiles: %v", err)
	}
	if got["/etc/foo.conf"] != "inline\ncontent" {
		t.Errorf("got[foo] = %q, want inline", got["/etc/foo.conf"])
	}
	if got["/etc/bar.conf"] != "bar content" {
		t.Errorf("got[bar] = %q, want %q", got["/etc/bar.conf"], "bar content")
	}
}

func TestConfigsDirFromYAMLPath(t *testing.T) {
	got := configsDirFromYAMLPath("/repo/packages/config/my-pkg.yaml", "my-pkg")
	want := "/repo/configs/my-pkg"
	if got != want {
		t.Errorf("configsDirFromYAMLPath = %q, want %q", got, want)
	}
}
