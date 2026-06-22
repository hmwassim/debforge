package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
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
}

func parseConfig(name string, data []byte) (*pkg.Package, error) {
	var def configDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse config definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:          name,
		Type:          pkg.TypeConfig,
		Depends:       def.Depends,
		Configs:       def.Install.Configs,
		RemoveConfigs: def.Remove.Configs,
	}, nil
}
