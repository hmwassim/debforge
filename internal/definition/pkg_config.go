package definition

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type configDefinition struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Depends []string `yaml:"depends,omitempty"`

	Install struct {
		Configs map[string]string `yaml:"configs,omitempty"`
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

	removeConfigs, err := resolveConfigFiles(def.Remove.Configs, fs, configsDir)
	if err != nil {
		return nil, fmt.Errorf("config definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:          name,
		Type:          pkg.TypeConfig,
		Depends:       def.Depends,
		Configs:       configs,
		RemoveConfigs: removeConfigs,
		PostInstall:   def.PostInstall,
		PostRemove:    def.PostRemove,
	}, nil
}

// configsDirFromYAMLPath derives the config source directory from the
// definition file path. Assumes the layout:
//
//	<root>/packages/config/<name>.yaml
//	<root>/configs/<name>/
func configsDirFromYAMLPath(yamlPath, pkgName string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(yamlPath)), "configs", pkgName)
}

// resolveConfigFiles reads config source files from configsDir and replaces
// filenames with their contents. If the source file doesn't exist, the
// value is kept as-is (allowing inline content alongside file references).
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
		srcPath := filepath.Join(configsDir, src)
		data, err := fs.ReadFile(srcPath)
		if err != nil {
			resolved[dest] = src
			continue
		}
		resolved[dest] = string(data)
	}
	return resolved, nil
}
