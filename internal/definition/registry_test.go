package definition

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

type walkErrorFS struct {
	ports.FileSystem
}

func (walkErrorFS) Walk(_ string, _ func(string, ports.FileInfo, error) error) error {
	return errors.New("walk failed")
}

func TestLoadAll_dirNotExist(t *testing.T) {
	t.Parallel()
	fs := testutil.NewMockFileSystem()
	reg := pkg.NewRegistry()
	err := LoadAll("/nonexistent", fs, reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}

func TestLoadAll_loadsYAMLFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg-a.yaml"), []byte(`
name: pkg-a
type: apt
install:
  packages:
    - hello
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg-b.yaml"), []byte(`
name: pkg-b
type: apt
install:
  packages:
    - world
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("not a yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if _, ok := reg.Lookup("pkg-a"); !ok {
		t.Error("pkg-a not registered")
	}
	if _, ok := reg.Lookup("pkg-b"); !ok {
		t.Error("pkg-b not registered")
	}
}

func TestLoadAll_badYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`{{{`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err == nil {
		t.Fatal("expected error from bad YAML")
	}
}

func TestLoadAll_walkError(t *testing.T) {
	t.Parallel()
	base := testutil.NewMockFileSystem()
	base.Files["/mydir"] = nil
	base.ExistsFunc = func(_ string) (bool, error) { return true, nil }
	fsys := walkErrorFS{FileSystem: base}
	reg := pkg.NewRegistry()
	err := LoadAll("/mydir", fsys, reg)
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func TestLoadAll_dirExistsButEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}
