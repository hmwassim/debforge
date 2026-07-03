package system

import (
	"os"
	"os/user"
	"testing"
)

func TestNewSystem(t *testing.T) {
	s := NewSystem()
	if s == nil {
		t.Fatal("NewSystem should not return nil")
	}
}

func TestIsPrivileged(t *testing.T) {
	want := os.Geteuid() == 0
	got := NewSystem().IsPrivileged()
	if got != want {
		t.Errorf("IsPrivileged() = %v, want %v (Geteuid() == 0)", got, want)
	}
}

func TestGetenv(t *testing.T) {
	key := "DEBFORGE_TEST_VAR"
	os.Setenv(key, "test-value")
	defer os.Unsetenv(key)

	got := NewSystem().Getenv(key)
	if got != "test-value" {
		t.Errorf("Getenv(%q) = %q, want %q", key, got, "test-value")
	}
}

func TestGetenv_missing(t *testing.T) {
	got := NewSystem().Getenv("DEBFORGE_NONEXISTENT_VAR_XYZ")
	if got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}
}

func TestUserHomeDir(t *testing.T) {
	dir, err := NewSystem().UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error: %v", err)
	}
	if dir == "" {
		t.Error("expected non-empty home directory")
	}
}

func TestLookupUser_current(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current(): %v", err)
	}
	info, err := NewSystem().LookupUser(current.Username)
	if err != nil {
		t.Fatalf("LookupUser(%q): %v", current.Username, err)
	}
	if info.HomeDir != current.HomeDir {
		t.Errorf("HomeDir = %q, want %q", info.HomeDir, current.HomeDir)
	}
}

func TestLookupUser_nonexistent(t *testing.T) {
	_, err := NewSystem().LookupUser("this-user-does-not-exist-xyzzy")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}
