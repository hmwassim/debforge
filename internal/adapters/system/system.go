// Package system provides a concrete implementation of ports.System
// by inspecting the host OS.
package system

import (
	"fmt"
	"os"
	"os/user"
	"strconv"

	"github.com/hmwassim/debforge/internal/ports"
)

// System implements ports.System for the host OS.
type System struct{}

// NewSystem returns a new System.
func NewSystem() *System {
	return &System{}
}

// IsPrivileged reports whether the process is running as root (UID 0).
func (s *System) IsPrivileged() bool {
	return os.Geteuid() == 0
}

// Getenv wraps os.Getenv.
func (s *System) Getenv(key string) string {
	return os.Getenv(key)
}

// UserHomeDir wraps os.UserHomeDir.
func (s *System) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// LookupUser wraps user.Lookup and converts string uid/gid to int.
func (s *System) LookupUser(name string) (*ports.UserInfo, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, errAtoi1 := strconv.Atoi(u.Uid)
	gid, errAtoi2 := strconv.Atoi(u.Gid)
	if errAtoi1 != nil || errAtoi2 != nil {
		return nil, fmt.Errorf("parse uid/gid for %s: %w / %w", name, errAtoi1, errAtoi2)
	}
	return &ports.UserInfo{HomeDir: u.HomeDir, Uid: uid, Gid: gid}, nil
}

var _ ports.System = (*System)(nil)
