package installer

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ConfigAction represents the decision from a three-way config file merge.
type ConfigAction int

const (
	ConfigWrite ConfigAction = iota
	ConfigSkip
	ConfigConflict
)

// DangerousRoots lists well-known system directories that must never be
// removed or overwritten by package operations. Both the config-path
// validator and the self-remove flow share this single source of truth.
var DangerousRoots = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64",
	"/opt", "/proc", "/root", "/run", "/sbin", "/sys", "/usr", "/var",
}

// allowedConfigPrefixes lists directory prefixes that config files may
// be written to. Paths outside these prefixes are rejected to prevent
// arbitrary filesystem writes from untrusted YAML definitions.
var allowedConfigPrefixes = []string{
	"/etc/",
	"/usr/share/",
	"/usr/lib/",
	"/opt/",
	"/boot/",
	"/var/",
}

// checkPathTraversal returns an error if path contains ".." components.
func checkPathTraversal(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path %q contains traversal component", path)
	}
	return nil
}

// ValidateConfigPath checks whether a config destination path falls
// within an allowed directory prefix and contains no traversal
// components. This prevents untrusted YAML definitions from writing to
// arbitrary filesystem locations.
func ValidateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if err := checkPathTraversal(path); err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("config path %q is not absolute", path)
	}
	for _, prefix := range allowedConfigPrefixes {
		if strings.HasPrefix(clean, prefix) {
			return nil
		}
	}
	return fmt.Errorf("config path %q is outside allowed directories", path)
}

// ValidateUserConfigPath checks that a user config path (after ~
// expansion) resolves within the given home directory.
func ValidateUserConfigPath(absPath, homeDir string) error {
	clean := filepath.Clean(absPath)
	if !strings.HasPrefix(clean, filepath.Clean(homeDir)+string(filepath.Separator)) {
		return fmt.Errorf("user config path %q escapes home directory %q", absPath, homeDir)
	}
	return nil
}

// ValidateRemovablePath returns an error if path is a dangerous system
// directory or contains traversal components.
func ValidateRemovablePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if err := checkPathTraversal(path); err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if clean == "/" {
		return fmt.Errorf("refusing to remove root directory")
	}
	for _, d := range DangerousRoots {
		if clean == d {
			return fmt.Errorf("refusing to remove %q: dangerous system path", clean)
		}
	}
	return nil
}
