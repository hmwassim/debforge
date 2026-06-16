package packages

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/writeutil"
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
	return executil.Run(exec.Command("apt-get", args...))
}

func DeployConfig(dest, content string, mode os.FileMode) error {
	if existing, err := os.ReadFile(dest); err == nil {
		contentMatch := string(existing) == content
		var modeMatch bool
		if fi, statErr := os.Stat(dest); statErr == nil {
			modeMatch = fi.Mode().Perm() == mode
		}
		if contentMatch && modeMatch {
			return nil
		}
		if contentMatch {
			return os.Chmod(dest, mode)
		}
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return writeutil.AtomicFile(dest, []byte(content), mode)
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
	return parseDpkgSelections(string(out), pkgs), nil
}

// parseDpkgSelections parses dpkg --get-selections output and returns a map of
// requested package names to installed status (true = "install" state).
//
// Architecture-qualified packages (e.g. "package:i386") are matched directly.
// When dpkg returns an architecture-qualified name for an unqualified request
// (e.g. "package:amd64" for requested "package"), the architecture suffix is
// stripped before matching. This ensures consistency regardless of how dpkg
// formats its output for the native architecture.
func parseDpkgSelections(out string, requested []string) map[string]bool {
	installed := make(map[string]bool, len(requested))
	for _, pkg := range requested {
		installed[pkg] = false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 || parts[1] != "install" {
			continue
		}
	name := parts[0]
	if _, ok := installed[name]; !ok {
		// Name from dpkg wasn't requested directly — strip the
		// architecture suffix (e.g. ":amd64", ":i386") and retry.
		if before, _, found := strings.Cut(name, ":"); found {
			name = before
		}
		if _, ok := installed[name]; !ok {
			continue
		}
	}
	installed[name] = true
	}
	return installed
}

func AptRemove(pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"purge", "-y", "--autoremove"}, pkgs...)
	return executil.Run(exec.Command("apt-get", args...))
}

func EnableService(name string) error {
	return executil.Run(exec.Command("systemctl", "enable", "--now", name))
}
