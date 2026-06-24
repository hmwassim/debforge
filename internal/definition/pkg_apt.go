package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type aptDefinition struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Depends []string `yaml:"depends,omitempty"`

	Install struct {
		Conflicts []string          `yaml:"conflicts,omitempty"`
		Extrepo   []string          `yaml:"extrepo,omitempty"`
		Backports []string          `yaml:"backports,omitempty"`
		Packages  []string          `yaml:"packages,omitempty"`
		Variants  map[string]string `yaml:"variants,omitempty"`
		Configs   map[string]string `yaml:"configs,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Packages []string          `yaml:"packages,omitempty"`
		Configs  map[string]string `yaml:"configs,omitempty"`
	} `yaml:"remove,omitempty"`

	PostInstall string `yaml:"post_install,omitempty"`
	PostRemove  string `yaml:"post_remove,omitempty"`
}

func parseApt(name string, data []byte) (*pkg.Package, error) {
	var def aptDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse apt definition %s: %w", name, err)
	}
	if len(def.Install.Packages) == 0 && len(def.Install.Variants) == 0 {
		return nil, fmt.Errorf("apt definition %s: no packages or variants defined", name)
	}

	return &pkg.Package{
		Name:      name,
		Type:      pkg.TypeApt,
		Depends:   def.Depends,
		Extrepo:   def.Install.Extrepo,
		Packages:  def.Install.Packages,
		Remove:    def.Remove.Packages,
		Backports: def.Install.Backports,
		Variants:  def.Install.Variants,
		Conflicts: def.Install.Conflicts,
		Configs:   def.Install.Configs,
		RemoveConfigs: def.Remove.Configs,
		PostInstall:   def.PostInstall,
		PostRemove:    def.PostRemove,
	}, nil
}
