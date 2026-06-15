package packages

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
)

func AptInstall(pkgs []string, backport bool, msg string, force bool) error {
	if len(pkgs) == 0 {
		return nil
	}
	args := []string{"install", "-y"}
	if backport {
		args = append(args, "-t", "trixie-backports")
	}
	args = append(args, pkgs...)
	cmd := exec.Command("apt-get", args...)
	if msg == "" {
		return executil.Run(cmd)
	}
	return executil.RunWithSpinner(cmd, msg)
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

func CheckInstalled(pkgs []string) (map[string]bool, error) {
	if len(pkgs) == 0 {
		return map[string]bool{}, nil
	}
	cmd := exec.Command("dpkg", append([]string{"--get-selections"}, pkgs...)...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		installed[pkg] = false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 || parts[1] != "install" {
			continue
		}
		name := parts[0]
		if _, ok := installed[name]; !ok {
			name, _, _ = strings.Cut(name, ":")
		}
		installed[name] = true
	}
	return installed, nil
}

func AptRemove(pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	return executil.Run(exec.Command("apt-get", append([]string{"remove", "-y"}, pkgs...)...))
}

func EnableService(name string, force bool) error {
	if force {
		executil.Run(exec.Command("systemctl", "disable", name))
	}
	return executil.Run(exec.Command("systemctl", "enable", "--now", name))
}
