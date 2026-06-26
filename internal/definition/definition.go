// Package definition parses YAML package definition files into pkg.Package
// values.
package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Parse reads a YAML definition from path and returns the parsed Package.
func Parse(path string, fs ports.FileSystem) (*pkg.Package, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read definition %s: %w", path, err)
	}

	var raw struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if raw.Name == "" {
		return nil, fmt.Errorf("definition %s: missing name", path)
	}

	switch raw.Type {
	case "apt":
		return parseApt(raw.Name, data)
	case "deb":
		return parseDeb(raw.Name, data)
	case "source":
		return parseSource(raw.Name, data)
	case "config":
		configsDir := configsDirFromYAMLPath(path, raw.Name)
		return parseConfig(raw.Name, data, fs, configsDir)
	default:
		return nil, fmt.Errorf("definition %s: unsupported type %q", path, raw.Type)
	}
}
