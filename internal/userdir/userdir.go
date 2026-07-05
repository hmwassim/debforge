// Package userdir provides utilities for resolving user home directory paths.
package userdir

import (
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// Home returns the home directory appropriate for the invoking user.
// When running under sudo (root with SUDO_USER set), it returns the original
// user's home directory so that ~ expansion in user_configs paths resolves
// to the real user's home (e.g. /home/wassim) rather than /root.
func Home(sys ports.System) (string, error) {
	if sudoUser := sys.Getenv("SUDO_USER"); sudoUser != "" && sys.IsPrivileged() {
		u, err := sys.LookupUser(sudoUser)
		if err == nil {
			return u.HomeDir, nil
		}
	}
	return sys.UserHomeDir()
}

// ExpandHome replaces a leading ~/ with homeDir, or returns homeDir for ~.
func ExpandHome(path, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	if path == "~" {
		return homeDir
	}
	return path
}

// HasPrefix reports whether path starts with ~/ or is exactly ~.
func HasPrefix(path string) bool {
	return strings.HasPrefix(path, "~/") || path == "~"
}
