package services

import (
	"context"
	"os"
	"testing"
)

func TestCoreStateLoadSave(t *testing.T) {
	fs := newMemFS()
	s := NewCoreStateStore(fs, &mockRunner{}, &mockUI{}, "/tmp/core.state.json")

	st := s.Load()
	if st.LastSetupCommit != "" {
		t.Fatalf("expected empty commit, got %q", st.LastSetupCommit)
	}

	st.LastSetupCommit = "abc123"
	st.ManagedPackages = []string{"pkg1", "pkg2"}
	st.ManagedConfigs = []string{"/etc/config1"}

	if err := s.Save(st); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded := s.Load()
	if loaded.LastSetupCommit != "abc123" {
		t.Fatalf("expected abc123, got %q", loaded.LastSetupCommit)
	}
	if len(loaded.ManagedPackages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(loaded.ManagedPackages))
	}
	if len(loaded.ManagedConfigs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(loaded.ManagedConfigs))
	}
}

func TestCoreStateLoadEmpty(t *testing.T) {
	fs := newMemFS()
	s := NewCoreStateStore(fs, &mockRunner{}, &mockUI{}, "/tmp/nonexistent.json")

	st := s.Load()
	if st.LastSetupCommit != "" {
		t.Fatal("expected empty state for missing file")
	}
}

func TestCoreStateLoadCorrupt(t *testing.T) {
	fs := newMemFS()
	fs.files["/tmp/core.state.json"] = []byte("{invalid json}")
	s := NewCoreStateStore(fs, &mockRunner{}, &mockUI{}, "/tmp/core.state.json")

	st := s.Load()
	if st.LastSetupCommit != "" {
		t.Fatal("expected reset state for corrupt file")
	}
}

func TestCoreStateCommitGitSuccess(t *testing.T) {
	runner := &mockRunner{stdout: []byte("abc123def\n")}
	s := NewCoreStateStore(newMemFS(), runner, &mockUI{}, "/tmp/core.state.json")

	commit := s.CurrentCommit(context.Background(), "/some/src")
	if commit != "abc123def" {
		t.Fatalf("expected abc123def, got %q", commit)
	}
}

func TestCoreStateCommitGitError(t *testing.T) {
	runner := &mockRunner{err: os.ErrNotExist}
	s := NewCoreStateStore(newMemFS(), runner, &mockUI{}, "/tmp/core.state.json")

	commit := s.CurrentCommit(context.Background(), "/some/src")
	if commit != "" {
		t.Fatalf("expected empty string on error, got %q", commit)
	}
}
