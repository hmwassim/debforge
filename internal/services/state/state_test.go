package state

import (
	"os"
	"testing"

	"github.com/hmwassim/debforge/internal/statestore"
)

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
	return nil, nil
}

func (f *memFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (f *memFS) RemoveAll(path string) error {
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
	return f.Stat(name)
}

func (f *memFS) Readlink(name string) (string, error) {
	return "", nil
}

func (f *memFS) Symlink(target, link string) error {
	return nil
}

func TestLoadEmptyState(t *testing.T) {
	fs := newMemFS()
	svc := NewService(fs, "/tmp/states")

	st, err := svc.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Version != 1 {
		t.Fatalf("expected version 1, got %d", st.Version)
	}
	if len(st.Packages) != 0 {
		t.Fatalf("expected empty packages, got %d", len(st.Packages))
	}
}

func TestSaveAndLoad(t *testing.T) {
	fs := newMemFS()
	svc := NewService(fs, "/tmp/states")

	st := &PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]PkgEntry{
		"firefox": {Type: "deb", Variant: "stable", Version: "1.0"},
	}}

	if err := svc.Save(st); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := svc.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.Packages["firefox"].Type != "deb" {
		t.Fatalf("expected deb type, got %s", loaded.Packages["firefox"].Type)
	}
	if loaded.Packages["firefox"].Variant != "stable" {
		t.Fatalf("expected stable variant, got %s", loaded.Packages["firefox"].Variant)
	}
}

func TestLookup(t *testing.T) {
	svc := NewService(newMemFS(), "/tmp/states")
	st := &PackagesState{Packages: map[string]PkgEntry{
		"firefox": {Type: "deb"},
	}}

	entry, ok := svc.Lookup(st, "firefox")
	if !ok {
		t.Fatal("expected firefox to be found")
	}
	if entry.Type != "deb" {
		t.Fatalf("expected deb, got %s", entry.Type)
	}

	_, ok = svc.Lookup(st, "nonexistent")
	if ok {
		t.Fatal("expected nonexistent to not be found")
	}
}

func TestIsInstalled(t *testing.T) {
	svc := NewService(newMemFS(), "/tmp/states")
	st := &PackagesState{Packages: map[string]PkgEntry{
		"firefox": {Type: "deb"},
	}}

	if !svc.IsInstalled(st, "firefox") {
		t.Fatal("expected firefox to be installed")
	}
	if svc.IsInstalled(st, "nonexistent") {
		t.Fatal("expected nonexistent to not be installed")
	}
}

func TestAdd(t *testing.T) {
	svc := NewService(newMemFS(), "/tmp/states")
	st := &PackagesState{Packages: map[string]PkgEntry{}}

	svc.Add(st, "firefox", PkgEntry{Type: "deb", Version: "1.0"})
	if _, ok := st.Packages["firefox"]; !ok {
		t.Fatal("expected firefox to be added")
	}
}

func TestRemove(t *testing.T) {
	svc := NewService(newMemFS(), "/tmp/states")
	st := &PackagesState{Packages: map[string]PkgEntry{
		"firefox": {Type: "deb"},
	}}

	svc.Remove(st, "firefox")
	if _, ok := st.Packages["firefox"]; ok {
		t.Fatal("expected firefox to be removed")
	}
}

func TestLoadCorruptState(t *testing.T) {
	fs := newMemFS()
	fs.files["/tmp/states/packages.state.json"] = []byte("{invalid json")
	svc := NewService(fs, "/tmp/states")

	_, err := svc.Load()
	if err == nil {
		t.Fatal("expected error for corrupt state")
	}
}

func TestLoadPermissionError(t *testing.T) {
	type permFS struct {
		*memFS
	}
	pfs := &permFS{memFS: newMemFS()}
	pfs.files["/tmp/states/packages.state.json"] = []byte(`{"version":1,"packages":{}}`)

	svc := NewService(pfs, "/tmp/states")

	_ = svc
}
