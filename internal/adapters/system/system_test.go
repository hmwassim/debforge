package system

import (
	"os"
	"testing"
)

func TestIsPrivileged(t *testing.T) {
	want := os.Geteuid() == 0
	got := NewSystem().IsPrivileged()
	if got != want {
		t.Errorf("IsPrivileged() = %v, want %v (Geteuid() == 0)", got, want)
	}
}
