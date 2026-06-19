package statestore

import (
	"os"
	"testing"
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

type testState struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

func TestLoadJSON(t *testing.T) {
	fs := newMemFS()
	s := New(fs)

	fs.files["/tmp/state.json"] = []byte(`{"version":1,"name":"test"}`)
	var st testState
	if err := s.LoadJSON("/tmp/state.json", &st); err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if st.Version != 1 || st.Name != "test" {
		t.Fatalf("got %+v, want {Version:1 Name:test}", st)
	}
}

func TestLoadJSONNotFound(t *testing.T) {
	s := New(newMemFS())
	var st testState
	err := s.LoadJSON("/tmp/nonexistent.json", &st)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadJSONCorrupt(t *testing.T) {
	fs := newMemFS()
	s := New(fs)

	fs.files["/tmp/state.json"] = []byte("{invalid")
	var st testState
	err := s.LoadJSON("/tmp/state.json", &st)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestSaveJSON(t *testing.T) {
	fs := newMemFS()
	s := New(fs)

	st := testState{Version: 1, Name: "hello"}
	if err := s.SaveJSON("/tmp/state.json", &st); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	data, err := fs.ReadFile("/tmp/state.json")
	if err != nil {
		t.Fatalf("file not saved: %v", err)
	}
	got := string(data)
	want := "{\n  \"version\": 1,\n  \"name\": \"hello\"\n}"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoad(t *testing.T) {
	fs := newMemFS()
	s := New(fs)

	fs.files["/tmp/state.json"] = []byte(`{"version":1,"name":"test"}`)
	var st testState
	found, err := s.Load("/tmp/state.json", &st)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if st.Name != "test" {
		t.Fatalf("got %+v", st)
	}
}

func TestLoadNotFound(t *testing.T) {
	s := New(newMemFS())
	var st testState
	found, err := s.Load("/tmp/nonexistent.json", &st)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Fatal("expected found=false for missing file")
	}
}

func TestLoadCorrupt(t *testing.T) {
	fs := newMemFS()
	s := New(fs)

	fs.files["/tmp/state.json"] = []byte("{invalid")
	var st testState
	_, err := s.Load("/tmp/state.json", &st)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestVersionedIsLegacy(t *testing.T) {
	v := &Versioned{}
	if !v.IsLegacy() {
		t.Fatal("expected IsLegacy()=true for Version 0")
	}
	v.Version = CurrentVersion
	if v.IsLegacy() {
		t.Fatal("expected IsLegacy()=false for CurrentVersion")
	}
}
