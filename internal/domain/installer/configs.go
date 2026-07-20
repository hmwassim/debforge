package installer

import (
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
	"github.com/hmwassim/debforge/internal/userdir"
)

// DecideConfigAction performs a three-way comparison between the on-disk
// file, the package definition's new content, and the stored baseline
// hash. Returns ConfigWrite when it's safe to overwrite, ConfigSkip when
// the user has local changes the package didn't touch, and ConfigConflict
// when both sides changed.
func DecideConfigAction(fs ports.FileSystem, path, newContent, baselineHash string, forceInstall bool) ConfigAction {
	if forceInstall {
		return ConfigWrite
	}
	exists, err := fs.Exists(path)
	if err != nil || !exists {
		return ConfigWrite
	}
	if baselineHash == "" {
		diskData, err := fs.ReadFile(path)
		if err != nil {
			return ConfigWrite
		}
		if string(diskData) == newContent {
			return ConfigSkip
		}
		return ConfigWrite
	}
	diskData, err := fs.ReadFile(path)
	if err != nil {
		return ConfigWrite
	}
	diskHash := textutil.Sha256Hex(diskData)
	newHash := textutil.Sha256Hex([]byte(newContent))

	switch {
	case diskHash == baselineHash && newHash == baselineHash:
		return ConfigWrite
	case diskHash == baselineHash && newHash != baselineHash:
		return ConfigWrite
	case diskHash != baselineHash && newHash == baselineHash:
		return ConfigSkip
	default:
		return ConfigConflict
	}
}

// WriteConfigFile writes content to path, creates parent directories, and
// returns the SHA-256 hash of the content.
// Callers must validate path before calling (see ValidateConfigPath for
// system configs, ValidateUserConfigPath for user configs).
func WriteConfigFile(fs ports.FileSystem, path, content string) (string, error) {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir %s: %w", dir, err)
	}
	if err := fs.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	return textutil.Sha256Hex([]byte(content)), nil
}

// WriteConfigSidecar writes content to path.debforge-new for conflict resolution.
func WriteConfigSidecar(fs ports.FileSystem, path, content string) error {
	sidecar := path + ".debforge-new"
	return fs.WriteFile(sidecar, []byte(content), 0644)
}

// WriteConfigs writes all config files defined in p.Configs.
// Backwards-compatible variant that does not track per-file hashes.
func WriteConfigs(fs ports.FileSystem, spinner ports.Spinner, p *pkg.Package) error {
	_, err := WriteConfigsWithHashes(fs, spinner, p, nil)
	return err
}

// WriteConfigsWithHashes writes config files with three-way merge
// protection. Returns the updated config hashes map keyed by absolute path.
func WriteConfigsWithHashes(fs ports.FileSystem, spinner ports.Spinner, p *pkg.Package, configHashes map[string]string) (map[string]string, error) {
	if len(p.Configs) == 0 {
		return configHashes, nil
	}
	if configHashes == nil {
		configHashes = make(map[string]string)
	}

	spinner.SetDesc("writing configs for " + p.Name)
	for path, content := range p.Configs {
		if err := ValidateConfigPath(path); err != nil {
			return nil, err
		}
		action := DecideConfigAction(fs, path, content, configHashes[path], p.ForceInstall)

		switch action {
		case ConfigWrite:
			hash, err := WriteConfigFile(fs, path, content)
			if err != nil {
				return nil, err
			}
			configHashes[path] = hash

		case ConfigSkip:
			if configHashes[path] == "" {
				diskData, err := fs.ReadFile(path)
				if err == nil && diskData != nil {
					configHashes[path] = textutil.Sha256Hex(diskData)
				}
			}

		case ConfigConflict:
			if err := WriteConfigSidecar(fs, path, content); err != nil {
				return nil, fmt.Errorf("write sidecar: %w", err)
			}
			spinner.SetDesc(fmt.Sprintf("%s has local changes; new version saved as %s — review and merge manually", path, path+".debforge-new"))
		}
	}
	return configHashes, nil
}

// WriteUserConfigs writes all user config files defined in p.UserConfigs.
// Backwards-compatible variant that does not track per-file hashes.
func WriteUserConfigs(fs ports.FileSystem, sys ports.System, spinner ports.Spinner, p *pkg.Package) error {
	_, err := WriteUserConfigsWithHashes(fs, sys, spinner, p, nil)
	return err
}

// WriteUserConfigsWithHashes writes user config files with three-way merge
// protection. Returns the updated config hashes map keyed by absolute
// (expanded) path.
func WriteUserConfigsWithHashes(fs ports.FileSystem, sys ports.System, spinner ports.Spinner, p *pkg.Package, configHashes map[string]string) (map[string]string, error) {
	if len(p.UserConfigs) == 0 {
		return configHashes, nil
	}
	if configHashes == nil {
		configHashes = make(map[string]string)
	}

	homeDir, err := userdir.Home(sys)
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	ownerUID, ownerGID, ownerChown := resolveSudoOwner(sys)

	spinner.SetDesc("writing user configs for " + p.Name)
	for path, content := range p.UserConfigs {
		absPath := userdir.ExpandHome(path, homeDir)
		if err := ValidateUserConfigPath(absPath, homeDir); err != nil {
			return nil, err
		}

		action := DecideConfigAction(fs, absPath, content, configHashes[absPath], p.ForceInstall)

		switch action {
		case ConfigWrite:
			hash, err := WriteConfigFile(fs, absPath, content)
			if err != nil {
				return nil, err
			}
			if ownerChown {
				dir := filepath.Dir(absPath)
				if err := fs.Chown(dir, ownerUID, ownerGID); err != nil {
					return nil, fmt.Errorf("chown config dir %s: %w", path, err)
				}
				if err := fs.Chown(absPath, ownerUID, ownerGID); err != nil {
					return nil, fmt.Errorf("chown config %s: %w", path, err)
				}
			}
			configHashes[absPath] = hash

		case ConfigSkip:
			if configHashes[absPath] == "" {
				diskData, err := fs.ReadFile(absPath)
				if err == nil && diskData != nil {
					configHashes[absPath] = textutil.Sha256Hex(diskData)
				}
			}

		case ConfigConflict:
			if err := WriteConfigSidecar(fs, absPath, content); err != nil {
				return nil, fmt.Errorf("write sidecar: %w", err)
			}
			if ownerChown {
				if err := fs.Chown(absPath+".debforge-new", ownerUID, ownerGID); err != nil {
					return nil, fmt.Errorf("chown sidecar %s: %w", path, err)
				}
			}
			spinner.SetDesc(fmt.Sprintf("%s has local changes; new version saved as %s — review and merge manually", absPath, absPath+".debforge-new"))
		}
	}
	return configHashes, nil
}

// resolveSudoOwner returns the uid/gid of the original user when running
// under sudo (root with SUDO_USER set), so that user config files written
// by debforge are owned by the invoking user rather than root.
func resolveSudoOwner(sys ports.System) (uid, gid int, ok bool) {
	if sudoUser := sys.Getenv("SUDO_USER"); sudoUser != "" && sys.IsPrivileged() {
		u, err := sys.LookupUser(sudoUser)
		if err != nil {
			return 0, 0, false
		}
		return u.Uid, u.Gid, true
	}
	return 0, 0, false
}

// WriteAllConfigs writes system configs and user configs with three-way
// merge protection, then updates p.ConfigHashes. This is the shared helper
// used by both the apt and config installer implementations.
func WriteAllConfigs(fs ports.FileSystem, sys ports.System, spinner ports.Spinner, p *pkg.Package) error {
	hashes := p.ConfigHashes
	if hashes == nil {
		hashes = make(map[string]string)
	}
	updated, err := WriteConfigsWithHashes(fs, spinner, p, hashes)
	if err != nil {
		return err
	}
	hashes = updated
	updated, err = WriteUserConfigsWithHashes(fs, sys, spinner, p, hashes)
	if err != nil {
		return err
	}
	p.ConfigHashes = updated
	return nil
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
