package self

import (
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestVerifyRemovablePath_empty(t *testing.T) {
	err := verifyRemovablePath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestVerifyRemovablePath_dangerous(t *testing.T) {
	dangerous := []string{"/", "/opt", "/usr", "/etc", "/var", "/home", "/root"}
	for _, d := range dangerous {
		t.Run(d, func(t *testing.T) {
			if err := verifyRemovablePath(d); err == nil {
				t.Errorf("expected error for dangerous path %q", d)
			}
		})
	}
}

func TestVerifyRemovablePath_safe(t *testing.T) {
	safe := []string{"/opt/debforge", "/tmp/foo", "/home/user/debforge"}
	for _, s := range safe {
		t.Run(s, func(t *testing.T) {
			if err := verifyRemovablePath(s); err != nil {
				t.Errorf("unexpected error for safe path %q: %v", s, err)
			}
		})
	}
}

func TestVerifyRemovablePath_cleanSafety(t *testing.T) {
	// /opt//debforge should clean to /opt/debforge (safe)
	if err := verifyRemovablePath("/opt//debforge"); err != nil {
		t.Errorf("unexpected error for /opt//debforge: %v", err)
	}
}

func TestSourceRepoExists_true(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	dir := "/opt/debforge/src"
	fs.Files[filepath.Join(dir, ".git")] = []byte{}

	ok, err := sourceRepoExists(fs, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected sourceRepoExists to return true")
	}
}

func TestSourceRepoExists_false(t *testing.T) {
	fs := testutil.NewMockFileSystem()

	ok, err := sourceRepoExists(fs, "/opt/debforge/src")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected sourceRepoExists to return false")
	}
}
