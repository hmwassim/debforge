package system

import (
	"os"

	"github.com/hmwassim/debforge/internal/ports"
)

type System struct{}

func NewSystem() *System {
	return &System{}
}

func (s *System) IsPrivileged() bool {
	return os.Geteuid() == 0
}

var _ ports.System = (*System)(nil)
