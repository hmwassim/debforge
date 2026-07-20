package userdir

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
)

type mockSystem struct {
	homeDir     string
	homeErr     error
	sudoUser    string
	privileged  bool
	userHome    string
	userErr     error
}

func (m *mockSystem) Getenv(_ string) string                              { return m.sudoUser }
func (m *mockSystem) UserHomeDir() (string, error)                        { return m.homeDir, m.homeErr }
func (m *mockSystem) IsPrivileged() bool                                  { return m.privileged }
func (m *mockSystem) LookupUser(_ string) (*ports.UserInfo, error)        { return &ports.UserInfo{HomeDir: m.userHome}, m.userErr }

func TestHome_regularUser(t *testing.T) {
	sys := &mockSystem{homeDir: "/home/testuser", privileged: false}
	home, err := Home(sys)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != "/home/testuser" {
		t.Errorf("got %q, want /home/testuser", home)
	}
}

func TestHome_sudoResolvesOriginalUser(t *testing.T) {
	sys := &mockSystem{
		sudoUser:   "realuser",
		privileged: true,
		userHome:   "/home/realuser",
		homeDir:    "/root",
	}
	home, err := Home(sys)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != "/home/realuser" {
		t.Errorf("got %q, want /home/realuser", home)
	}
}

func TestHome_sudoUserLookupFallsBack(t *testing.T) {
	sys := &mockSystem{
		sudoUser:   "unknown",
		privileged: true,
		userErr:    context.Canceled,
		homeDir:    "/root",
	}
	home, err := Home(sys)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != "/root" {
		t.Errorf("got %q, want /root", home)
	}
}

func TestHome_notPrivilegedIgnoresSudoUser(t *testing.T) {
	sys := &mockSystem{
		sudoUser:   "realuser",
		privileged: false,
		homeDir:    "/root",
	}
	home, err := Home(sys)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if home != "/root" {
		t.Errorf("got %q, want /root", home)
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		path    string
		homeDir string
		want    string
	}{
		{"~/docs", "/home/user", "/home/user/docs"},
		{"~", "/home/user", "/home/user"},
		{"/etc/config", "/home/user", "/etc/config"},
		{"relative/path", "/home/user", "relative/path"},
	}
	for _, tt := range tests {
		if got := ExpandHome(tt.path, tt.homeDir); got != tt.want {
			t.Errorf("ExpandHome(%q, %q) = %q, want %q", tt.path, tt.homeDir, got, tt.want)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"~/docs", true},
		{"~", true},
		{"/etc/config", false},
		{"relative", false},
	}
	for _, tt := range tests {
		if got := HasPrefix(tt.path); got != tt.want {
			t.Errorf("HasPrefix(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
