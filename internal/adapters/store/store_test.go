package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

type testData struct {
	Name string `json:"name"`
	Val  int    `json:"val"`
}

func TestLoad_notFound(t *testing.T) {
	dir := t.TempDir()

	s := NewStore[testData](fs.NewFileSystem(), filepath.Join(dir, "nonexistent.json"))
	_, err := s.Load()
	if err != ErrNotFound {
		t.Fatalf("Load = %v, want ErrNotFound", err)
	}
}

func TestSaveLoad_roundTrip(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "state.json")
	s := NewStore[testData](fs.NewFileSystem(), path)

	data := &testData{Name: "hello", Val: 42}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "hello" || loaded.Val != 42 {
		t.Errorf("Load = %+v, want {hello 42}", loaded)
	}
}

func TestSaveLoad_zeroValue(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "state.json")
	s := NewStore[testData](fs.NewFileSystem(), path)

	data := &testData{}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "" || loaded.Val != 0 {
		t.Errorf("Load = %+v, want zero value", loaded)
	}
}

func TestSaveLoad_nestedDir(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "sub", "dir", "state.json")
	s := NewStore[testData](fs.NewFileSystem(), path)

	data := &testData{Name: "nested"}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save to nested path: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load from nested path: %v", err)
	}
	if loaded.Name != "nested" {
		t.Errorf("Load = %+v, want {nested 0}", loaded)
	}
}

// ---- Load error paths ------------------------------------------------------

func TestLoad_existsError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) { return false, errors.New("stat failed") }
	s := NewStore[testData](fsys, "/state.json")
	_, err := s.Load()
	if err == nil {
		t.Fatal("expected error from Exists")
	}
}

func TestLoad_readError(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.ExistsFunc = func(_ string) (bool, error) { return true, nil }
	s := NewStore[testData](fsys, "/nonexistent.json")
	_, err := s.Load()
	if err == nil {
		t.Fatal("expected error from ReadFile")
	}
}

func TestLoad_corruptJSON(t *testing.T) {
	fsys := testutil.NewMockFileSystem()
	fsys.Files["/state.json"] = []byte("{invalid")
	s := NewStore[testData](fsys, "/state.json")
	_, err := s.Load()
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// ---- Save error paths ------------------------------------------------------

type errorFS struct {
	ports.FileSystem
	mkdirAllErr  error
	writeFileErr error
	renameErr    error
}

func (e errorFS) MkdirAll(_ string, _ int) error            { return e.mkdirAllErr }
func (e errorFS) WriteFile(_ string, _ []byte, _ int) error { return e.writeFileErr }
func (e errorFS) Rename(_, _ string) error                  { return e.renameErr }

func TestSave_mkdirAllError(t *testing.T) {
	base := fs.NewFileSystem()
	dir := t.TempDir()
	s := NewStore[testData](errorFS{FileSystem: base, mkdirAllErr: errors.New("mkdir failed")}, filepath.Join(dir, "sub", "state.json"))
	if err := s.Save(&testData{Name: "x"}); err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestSave_writeFileError(t *testing.T) {
	base := fs.NewFileSystem()
	dir := t.TempDir()
	s := NewStore[testData](errorFS{FileSystem: base, writeFileErr: errors.New("write failed")}, filepath.Join(dir, "state.json"))
	if err := s.Save(&testData{Name: "x"}); err == nil {
		t.Fatal("expected WriteFile error")
	}
}

func TestSave_renameError(t *testing.T) {
	base := fs.NewFileSystem()
	dir := t.TempDir()
	s := NewStore[testData](errorFS{FileSystem: base, renameErr: errors.New("rename failed")}, filepath.Join(dir, "state.json"))
	if err := s.Save(&testData{Name: "x"}); err == nil {
		t.Fatal("expected Rename error")
	}
}

func TestSave_marshalError(t *testing.T) {
	type unJSONable struct {
		Ch chan int
	}
	s := NewStore[unJSONable](fs.NewFileSystem(), filepath.Join(t.TempDir(), "x.json"))
	if err := s.Save(&unJSONable{Ch: make(chan int)}); err == nil {
		t.Fatal("expected JSON marshal error")
	}
}

func TestSave_overwrites(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "state.json")
	s := NewStore[testData](fs.NewFileSystem(), path)

	if err := s.Save(&testData{Name: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(&testData{Name: "second"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "second" {
		t.Errorf("Load after overwrite = %+v, want {second 0}", loaded)
	}
}
