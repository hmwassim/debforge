package self

import (
	"fmt"

	"github.com/hmwassim/debforge/internal/ports"
)

func requireRoot(action string, sys ports.System) error {
	if !sys.IsPrivileged() {
		return fmt.Errorf("--%s must be run as root", action)
	}
	return nil
}
