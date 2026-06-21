package self

import (
	"fmt"
	"os"
)

func requireRoot(action string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--%s must be run as root", action)
	}
	return nil
}
