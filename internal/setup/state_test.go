package setup

import (
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- State tests -----------------------------------------------------------

func TestLoadState_NotFound(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	st, err := LoadState(fs, "/nonexistent/setup_state.json")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if st.ConfigHashes == nil {
		t.Error("ConfigHashes should be initialized")
	}
}

func TestLoadState_Existing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	path := "/var/setup_state.json"
	fs.Files[path] = []byte(`{"config_hashes":{"/etc/foo.conf":"abc123"}}`)
	st, err := LoadState(fs, path)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if st.ConfigHashes["/etc/foo.conf"] != "abc123" {
		t.Errorf("expected abc123, got %q", st.ConfigHashes["/etc/foo.conf"])
	}
}

func TestSaveAndLoadState(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	path := "/var/setup_state.json"
	st := &State{ConfigHashes: map[string]string{"/a": "hash1", "/b": "hash2"}}
	if err := SaveState(fs, path, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadState(fs, path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ConfigHashes["/a"] != "hash1" {
		t.Errorf("expected hash1, got %q", loaded.ConfigHashes["/a"])
	}
	if loaded.ConfigHashes["/b"] != "hash2" {
		t.Errorf("expected hash2, got %q", loaded.ConfigHashes["/b"])
	}
}
