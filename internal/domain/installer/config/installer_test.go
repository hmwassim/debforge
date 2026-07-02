package config

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

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

func TestInstall_skipsWhenHashMatches(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written on hash match, got %d files", len(fs.files))
	}
	if p.Version != hash {
		t.Errorf("expected version unchanged on hash match, got %q", p.Version)
	}
}

func TestInstall_writesConfigsOnFirstInstall(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 1 {
		t.Fatalf("expected 1 file written, got %d", len(fs.files))
	}
	if string(fs.files["/etc/foo.conf"]) != "content" {
		t.Errorf("expected file content %q, got %q", "content", string(fs.files["/etc/foo.conf"]))
	}
	if p.Version == "" {
		t.Error("expected version to be set after install")
	}
}

func TestInstall_updatesVersionOnConfigChange(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}

	oldHash := computeConfigHash(&pkg.Package{
		Configs: map[string]string{"/etc/foo.conf": "old"},
	})

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: oldHash,
		Configs: map[string]string{"/etc/foo.conf": "old"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written when content unchanged, got %d files", len(fs.files))
	}
	if p.Version != oldHash {
		t.Errorf("expected version unchanged, got %q", p.Version)
	}

	// Now change config content
	p.Configs["/etc/foo.conf"] = "new content"
	err = inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install after config change: %v", err)
	}

	if string(fs.files["/etc/foo.conf"]) != "new content" {
		t.Errorf("expected updated file content %q, got %q", "new content", string(fs.files["/etc/foo.conf"]))
	}
	newHash := p.Version
	if newHash == oldHash {
		t.Error("expected version to change after config change")
	}
}

func TestInstall_forceBypassesHashCheck(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}

	p := &pkg.Package{
		Name:         "test-config",
		Type:         pkg.TypeConfig,
		ForceInstall: true,
		Configs:      map[string]string{"/etc/foo.conf": "content"},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 1 {
		t.Errorf("expected 1 file written with ForceInstall, got %d files", len(fs.files))
	}
	if p.Version != hash {
		t.Errorf("expected version unchanged after force install, got %q", p.Version)
	}
}

func TestInstall_includesUserConfigsInHash(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written on hash match with user configs, got %d files", len(fs.files))
	}
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

	// Same data different insertion order should produce same hash
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

func TestInstall_wrongType(t *testing.T) {
	inst := &Installer{fs: newMockFS()}
	p := &pkg.Package{Name: "test", Type: pkg.TypeApt}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error for non-config type")
	}
}

func TestInstall_withUserConfigs(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs}
	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "system content"},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if string(fs.files["/etc/foo.conf"]) != "system content" {
		t.Errorf("system config not written correctly, got %q", string(fs.files["/etc/foo.conf"]))
	}

	homeDir, err := installer.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expandedPath := filepath.Join(homeDir, ".config/bar.conf")
	if string(fs.files[expandedPath]) != "user content" {
		t.Errorf("user config not written at %s, got %q", expandedPath, string(fs.files[expandedPath]))
	}
}

func TestRemove_configs(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	homeDir, err := installer.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Put file at expanded path with matching content so removal proceeds
	fs.files[filepath.Join(homeDir, ".config/bar.conf")] = []byte("user content")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
		RemoveConfigs: map[string]string{
			"/etc/removed.conf": "",
		},
		Configs: map[string]string{
			"/etc/foo.conf": "content",
		},
	}
	inst := &Installer{fs: fs}
	err = inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	expandedPath := filepath.Join(homeDir, ".config/bar.conf")
	expected := []string{expandedPath, "/etc/removed.conf", "/etc/foo.conf"}
	for _, e := range expected {
		found := false
		for _, r := range removed {
			if r == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be removed, got %v", e, removed)
		}
	}
}

func TestRemove_skipModifiedUserConfig(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	homeDir, err := installer.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// File exists with different content => FileIsModified returns true => skip
	fs.files[filepath.Join(homeDir, ".config/bar.conf")] = []byte("modified content")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "original content",
		},
	}
	inst := &Installer{fs: fs}
	err = inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals for modified user config, got %v", removed)
	}
}

func TestRemove_removeAllError(t *testing.T) {
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		return fmt.Errorf("remove failed")
	}
	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}
	inst := &Installer{fs: fs}
	err := inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error from RemoveAll")
	}
}
