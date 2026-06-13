package executil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func Run(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf(s)
		}
		return err
	}
	return nil
}
