// Package system provides a concrete implementation of ports.System
// by inspecting the host OS.
package system

import (
	"os"

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

var _ ports.System = (*System)(nil)
