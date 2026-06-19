package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteFile(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := fs.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := fs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", string(data))
	}
}

func TestAtomicWriteFile(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")

	if err := fs.AtomicWriteFile(path, []byte("atomic"), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := fs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "atomic" {
		t.Fatalf("expected 'atomic', got '%s'", string(data))
	}
}

func TestMkdirAllAndRemoveAll(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b", "c")

	if err := fs.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(subdir); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveAll(subdir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(subdir); !os.IsNotExist(err) {
		t.Fatal("expected subdir to be removed")
	}
}

func TestRename(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := fs.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("expected src to be gone after rename")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("expected dst to exist after rename")
	}
}

func TestStat(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "stat.txt")

	// stat non-existent
	if _, err := fs.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected not-exist error")
	}

	if err := fs.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	fi, err := fs.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 4 {
		t.Fatalf("expected size 4, got %d", fi.Size())
	}
}

func TestChmod(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "chmod.txt")

	if err := fs.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fs.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	fi, err := fs.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600, got %o", fi.Mode().Perm())
	}
}

func TestMkdirTemp(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()

	tmpDir, err := fs.MkdirTemp(dir, "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveAll(tmpDir)

	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatal("expected temp dir to exist")
	}
}

func TestReadDir(t *testing.T) {
	fs := NewOSFileSystem()
	dir := t.TempDir()

	if err := fs.WriteFile(filepath.Join(dir, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(filepath.Join(dir, "b.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}
