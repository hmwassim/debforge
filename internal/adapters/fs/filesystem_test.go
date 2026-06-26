package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

func TestWriteFile_ReadFile_roundtrip(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := fsys.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	data, err := fsys.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadFile() = %q, want %q", string(data), "hello")
	}
}

func TestRemoveAll_file(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "remove-me")

	if err := fsys.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.RemoveAll(path); err != nil {
		t.Fatalf("RemoveAll() = %v", err)
	}
	exists, err := fsys.Exists(path)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("file still exists after RemoveAll")
	}
}

func TestRemoveAll_directory(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := fsys.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "nested.txt")
	if err := fsys.WriteFile(nested, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := fsys.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll() = %v", err)
	}
	exists, err := fsys.Exists(dir)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("directory still exists after RemoveAll")
	}
}

func TestMkdirAll(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")

	if err := fsys.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("MkdirAll() = %v", err)
	}
	fi, err := os.Stat(deep)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Error("expected directory")
	}
}

func TestMkdirTemp(t *testing.T) {
	fsys := NewFileSystem()
	created, err := fsys.MkdirTemp("debforge-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp() = %v", err)
	}
	defer os.RemoveAll(created)

	if !strings.Contains(created, "debforge-test-") {
		t.Errorf("created path = %q, want prefix debforge-test-", created)
	}
	fi, err := os.Stat(created)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Error("expected directory")
	}
}

func TestCreate(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "created.txt")

	wc, err := fsys.Create(path)
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if _, err := wc.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Errorf("content = %q, want %q", string(data), "data")
	}
}

func TestStat(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "stat-me")
	if err := fsys.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	fi, err := fsys.Stat(path)
	if err != nil {
		t.Fatalf("Stat() = %v", err)
	}
	if fi.Name() != "stat-me" {
		t.Errorf("Name() = %q, want %q", fi.Name(), "stat-me")
	}
	if fi.Size() != 7 {
		t.Errorf("Size() = %d, want %d", fi.Size(), 7)
	}
	if fi.IsDir() {
		t.Error("IsDir() = true, want false")
	}
}

func TestStat_notFound(t *testing.T) {
	fsys := NewFileSystem()
	_, err := fsys.Stat(filepath.Join(t.TempDir(), "no-such-file"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGlob(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.go"} {
		if err := fsys.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	matches, err := fsys.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatalf("Glob() = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Glob() = %v, want 2 matches", matches)
	}
}

func TestRename(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")

	if err := fsys.WriteFile(oldPath, []byte("rename me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename() = %v", err)
	}
	exists, _ := fsys.Exists(oldPath)
	if exists {
		t.Error("old file still exists after rename")
	}
	data, err := fsys.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "rename me" {
		t.Errorf("content = %q, want %q", string(data), "rename me")
	}
}

func TestSymlink_Readlink(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	if err := fsys.WriteFile(target, []byte("link target"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() = %v", err)
	}

	got, err := fsys.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink() = %v", err)
	}
	if got != target {
		t.Errorf("Readlink() = %q, want %q", got, target)
	}
}

func TestWalk(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	if err := fsys.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(filepath.Join(dir, "root.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(filepath.Join(dir, "sub", "nested.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	var visited []string
	err := fsys.Walk(dir, func(path string, info ports.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		visited = append(visited, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() = %v", err)
	}

	if len(visited) != 4 {
		t.Fatalf("Walk visited %d entries, want 4: %v", len(visited), visited)
	}
}

func TestExists_true(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "exists")
	if err := fsys.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := fsys.Exists(path)
	if err != nil {
		t.Fatalf("Exists() = %v", err)
	}
	if !got {
		t.Error("Exists() = false, want true")
	}
}

func TestExists_false(t *testing.T) {
	fsys := NewFileSystem()
	got, err := fsys.Exists(filepath.Join(t.TempDir(), "no-such-file"))
	if err != nil {
		t.Fatalf("Exists() = %v", err)
	}
	if got {
		t.Error("Exists() = true, want false")
	}
}

func TestReadFile_notFound(t *testing.T) {
	fsys := NewFileSystem()
	_, err := fsys.ReadFile(filepath.Join(t.TempDir(), "no-such-file"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSymlink_Readlink_notSymlink(t *testing.T) {
	fsys := NewFileSystem()
	dir := t.TempDir()
	path := filepath.Join(dir, "regular")
	if err := fsys.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := fsys.Readlink(path)
	if err == nil {
		t.Fatal("expected error for non-symlink")
	}
}
