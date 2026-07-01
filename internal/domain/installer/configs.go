package installer

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// WriteConfigs writes all config files defined in p.Configs.
func WriteConfigs(fs ports.FileSystem, spinner ports.Spinner, p *pkg.Package) error {
	if len(p.Configs) == 0 {
		return nil
	}

	spinner.SetDesc("writing configs for " + p.Name)
	for path, content := range p.Configs {
		dir := filepath.Dir(path)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
		if err := fs.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write config %s: %w", path, err)
		}
	}
	return nil
}

// WriteUserConfigs writes all user config files defined in p.UserConfigs.
// Paths starting with ~/ are expanded to the user's home directory.
func WriteUserConfigs(fs ports.FileSystem, spinner ports.Spinner, p *pkg.Package) error {
	if len(p.UserConfigs) == 0 {
		return nil
	}

	homeDir, err := UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	ownerUID, ownerGID, ownerChown := resolveSudoOwner()

	spinner.SetDesc("writing user configs for " + p.Name)
	for path, content := range p.UserConfigs {
		path = ExpandHome(path, homeDir)

		if FileIsModified(fs, path, content, p.ForceInstall) {
			spinner.SetDesc("skipping modified user config " + path)
			continue
		}

		dir := filepath.Dir(path)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create user config dir %s: %w", dir, err)
		}
		if ownerChown {
			if err := fs.Chown(dir, ownerUID, ownerGID); err != nil {
				return fmt.Errorf("chown config dir %s: %w", dir, err)
			}
		}
		if err := fs.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write user config %s: %w", path, err)
		}
		if ownerChown {
			if err := fs.Chown(path, ownerUID, ownerGID); err != nil {
				return fmt.Errorf("chown config %s: %w", path, err)
			}
		}
	}
	return nil
}

// UserHomeDir returns the home directory appropriate for the invoking user.
// When running under sudo (root with SUDO_USER set), it returns the original
// user's home directory so that ~ expansion in user_configs paths resolves
// to the real user's home (e.g. /home/wassim) rather than /root.
func UserHomeDir() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && os.Geteuid() == 0 {
		u, err := user.Lookup(sudoUser)
		if err == nil {
			return u.HomeDir, nil
		}
	}
	return os.UserHomeDir()
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

// resolveSudoOwner returns the uid/gid of the original user when running
// under sudo (root with SUDO_USER set), so that user config files written
// by debforge are owned by the invoking user rather than root.
func resolveSudoOwner() (uid, gid int, ok bool) {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && os.Geteuid() == 0 {
		u, err := user.Lookup(sudoUser)
		if err != nil {
			return 0, 0, false
		}
		uid, errAtoi1 := strconv.Atoi(u.Uid)
		gid, errAtoi2 := strconv.Atoi(u.Gid)
		if errAtoi1 != nil || errAtoi2 != nil {
			return 0, 0, false
		}
		return uid, gid, true
	}
	return 0, 0, false
}

// HasHomePrefix reports whether path starts with ~/ or is exactly ~.
func HasHomePrefix(path string) bool {
	return strings.HasPrefix(path, "~/") || path == "~"
}

// FileIsModified reports whether the file at path exists and its content
// differs from want. Returns false when the file does not exist, when it
// matches, or when ForceInstall is true (which skips the check entirely).
func FileIsModified(fs ports.FileSystem, path string, want string, forceInstall bool) bool {
	if forceInstall {
		return false
	}
	ok, err := fs.Exists(path)
	if err != nil || !ok {
		return false
	}
	existing, err := fs.ReadFile(path)
	if err != nil {
		return false
	}
	return string(existing) != want
}
