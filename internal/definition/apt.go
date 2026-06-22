package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type aptDefinition struct {
	Name      string            `yaml:"name"`
	Type      string            `yaml:"type"`
	Depends   []string          `yaml:"depends,omitempty"`
	Extrepo   []string          `yaml:"extrepo,omitempty"`
	Packages  []string          `yaml:"packages,omitempty"`
	Backports []string          `yaml:"backports,omitempty"`
	Variants  map[string]string `yaml:"variants,omitempty"`
	Conflicts []string          `yaml:"conflicts,omitempty"`
	Primary   string            `yaml:"primary,omitempty"`
	Configs   map[string]string `yaml:"configs,omitempty"`
}

func parseApt(name string, data []byte) (*pkg.Package, error) {
	var def aptDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse apt definition %s: %w", name, err)
	}
	if len(def.Packages) == 0 && len(def.Variants) == 0 {
		return nil, fmt.Errorf("apt definition %s: no packages or variants defined", name)
	}

	primary := def.Primary
	if primary == "" {
		if len(def.Packages) > 0 {
			primary = def.Packages[0]
		} else {
			for _, v := range def.Variants {
				primary = v
				break
			}
		}
	}

	return &pkg.Package{
		Name:      name,
		Type:      pkg.TypeApt,
		Depends:   def.Depends,
		Extrepo:   def.Extrepo,
		Packages:  def.Packages,
		Primary:   primary,
		Backports: def.Backports,
		Variants:  def.Variants,
		Conflicts: def.Conflicts,
		Configs:   def.Configs,
	}, nil
}
