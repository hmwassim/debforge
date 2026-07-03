package definition

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type configDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Type        string   `yaml:"type"`
	Depends     []string `yaml:"depends,omitempty"`

	Install struct {
		Configs     map[string]string `yaml:"configs,omitempty"`
		UserConfigs map[string]string `yaml:"user_configs,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Configs map[string]string `yaml:"configs,omitempty"`
	} `yaml:"remove,omitempty"`

	PostInstall string `yaml:"post_install,omitempty"`
	PostRemove  string `yaml:"post_remove,omitempty"`
}

func parseConfig(name string, data []byte, fs ports.FileSystem, configsDir string) (*pkg.Package, error) {
	var def configDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse config definition %s: %w", name, err)
	}

	configs, err := resolveConfigFiles(def.Install.Configs, fs, configsDir)
	if err != nil {
		return nil, fmt.Errorf("config definition %s: %w", name, err)
	}

	userConfigs, err := resolveConfigFiles(def.Install.UserConfigs, fs, configsDir)
	if err != nil {
		return nil, fmt.Errorf("config definition %s: %w", name, err)
	}

	// Remove configs are destination-path-only; values are cosmetic.
	removeConfigs := def.Remove.Configs
	if removeConfigs != nil {
		rm := make(map[string]string, len(removeConfigs))
		for k := range removeConfigs {
			rm[k] = ""
		}
		removeConfigs = rm
	}

	postInstall, err := resolveScriptFile(def.PostInstall, fs, configsDir)
	if err != nil {
		return nil, fmt.Errorf("config definition %s: %w", name, err)
	}

	postRemove, err := resolveScriptFile(def.PostRemove, fs, configsDir)
	if err != nil {
		return nil, fmt.Errorf("config definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:          name,
		Description:   def.Description,
		Type:          pkg.TypeConfig,
		Depends:       def.Depends,
		Configs:       configs,
		RemoveConfigs: removeConfigs,
		UserConfigs:   userConfigs,
		PostInstall:   postInstall,
		PostRemove:    postRemove,
	}, nil
}

// resolveScriptFile reads a script from a file in configsDir if the value
// looks like a filename (no newlines). Inline scripts (with newlines) and
// empty values are returned as-is. If the file doesn't exist, the value is
// treated as an inline script.
func resolveScriptFile(script string, fs ports.FileSystem, configsDir string) (string, error) {
	if script == "" || containsNewline(script) {
		return script, nil
	}
	srcPath := filepath.Join(configsDir, script)
	cleanDir := filepath.Clean(configsDir)
	if !strings.HasPrefix(filepath.Clean(srcPath), cleanDir+string(filepath.Separator)) && filepath.Clean(srcPath) != cleanDir {
		return "", fmt.Errorf("script source %s: path traversal outside configs directory", script)
	}
	data, err := fs.ReadFile(srcPath)
	if err != nil {
		return script, nil
	}
	return string(data), nil
}

// configsDirFromYAMLPath derives the config source directory from the
// definition file path. Assumes the layout:
//
//	<root>/packages/config/<name>.yaml
//	<root>/configs/<name>/
func configsDirFromYAMLPath(yamlPath, pkgName string) string {
	// yamlPath = .../packages/config/<name>.yaml
	// want:    .../configs/<name>/
	packagesDir := filepath.Dir(filepath.Dir(yamlPath)) // .../packages/
	repoDir := filepath.Dir(packagesDir)                // .../
	return filepath.Join(repoDir, "configs", pkgName)
}

// resolveConfigFiles reads config source files from configsDir and replaces
// filenames with their contents. Values that contain newlines are treated as
// inline content and kept as-is. Plain filenames are resolved relative to
// configsDir and error if the file cannot be read.
func resolveConfigFiles(raw map[string]string, fs ports.FileSystem, configsDir string) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	resolved := make(map[string]string, len(raw))
	for dest, src := range raw {
		if src == "" {
			resolved[dest] = src
			continue
		}
		if containsNewline(src) {
			resolved[dest] = src
			continue
		}
		srcPath := filepath.Join(configsDir, src)
		cleanDir := filepath.Clean(configsDir)
		if !strings.HasPrefix(filepath.Clean(srcPath), cleanDir+string(filepath.Separator)) && filepath.Clean(srcPath) != cleanDir {
			return nil, fmt.Errorf("config source %s: path traversal outside configs directory", src)
		}
		data, err := fs.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("read config source %s for %s: %w", srcPath, dest, err)
		}
		resolved[dest] = string(data)
	}
	return resolved, nil
}

func containsNewline(s string) bool {
	for i := range s {
		if s[i] == '\n' {
			return true
		}
	}
	return false
}
