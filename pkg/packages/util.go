package packages

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
)

func AptInstall(pkgs []string, backport bool) error {
	if len(pkgs) == 0 {
		return nil
	}
	args := []string{"install", "-y"}
	if backport {
		args = append(args, "-t", "trixie-backports")
	}
	args = append(args, pkgs...)
	return executil.Run(exec.Command("apt", args...))
}

func DeployConfig(dest, content string, mode os.FileMode) error {
	i := strings.LastIndex(dest, "/")
	if i < 0 {
		return fmt.Errorf("invalid config path: %q", dest)
	}
	if err := os.MkdirAll(dest[:i], 0755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(content), mode)
}

func IsInstalled(pkg string) bool {
	out, err := exec.Command("dpkg", "--get-selections", pkg).Output()
	return err == nil && strings.Contains(string(out), "\tinstall")
}

func EnableService(name string) error {
	return executil.Run(exec.Command("systemctl", "enable", "--now", name))
}
