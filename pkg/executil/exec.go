package executil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Run(cmd *exec.Cmd) error {
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w", s, err)
		}
		return err
	}
	return nil
}
