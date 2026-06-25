package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
)

type testData struct {
	Name string `json:"name"`
	Val  int    `json:"val"`
}

func TestLoad_notFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s := NewStore[testData](fs.NewFileSystem(), filepath.Join(dir, "nonexistent.json"))
	_, err = s.Load()
	if err != ErrNotFound {
		t.Fatalf("Load = %v, want ErrNotFound", err)
	}
}

func TestSaveLoad_roundTrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

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
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

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
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

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

func TestSave_overwrites(t *testing.T) {
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

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
