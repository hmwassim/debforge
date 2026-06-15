package packages

import (
	"bytes"
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
	if force {
		args = append(args, "--reinstall")
	}
	if backport {
		args = append(args, "-t", "trixie-backports")
	}
	args = append(args, pkgs...)
	cmd := exec.Command("apt", args...)
	if msg == "" {
		cmd.Stdout = io.Discard
		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if err := cmd.Run(); err != nil {
			if s := strings.TrimSpace(stderr.String()); s != "" {
				return fmt.Errorf("%s: %w", s, err)
			}
			return fmt.Errorf("apt install: %w", err)
		}
		return nil
	}
	return executil.RunWithSpinner(cmd, msg)
}

func DeployConfig(dest, content string, mode os.FileMode) error {
	if existing, err := os.ReadFile(dest); err == nil && string(existing) == content {
		return nil
	}
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
	cmd := exec.Command("apt", append([]string{"remove", "-y"}, pkgs...)...)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return fmt.Errorf("%s: %w", s, err)
		}
		return fmt.Errorf("apt remove: %w", err)
	}
	return nil
}

func EnableService(name string, force bool) error {
	if !force {
		if executil.Run(exec.Command("systemctl", "is-enabled", "--quiet", name)) == nil &&
			executil.Run(exec.Command("systemctl", "is-active", "--quiet", name)) == nil {
			return nil
		}
	} else {
		executil.Run(exec.Command("systemctl", "disable", name))
	}
	return executil.Run(exec.Command("systemctl", "enable", "--now", name))
}
