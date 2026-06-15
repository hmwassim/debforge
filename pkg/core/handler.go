package core

import (
	"fmt"

	"github.com/hmwassim/debforge/pkg/packages"
)

func init() {
	packages.Register(Handler{})
}

type Handler struct{}

func (Handler) Type() string { return "core" }

func (Handler) Install(string) error {
	return fmt.Errorf("core packages are managed via 'debforge core setup' or 'debforge core update'")
}

func (Handler) Remove(string) error {
	return fmt.Errorf("core packages cannot be removed")
}
