package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type aptDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Categories  []string `yaml:"categories,omitempty"`
	Type        string   `yaml:"type"`
	Depends     []string `yaml:"depends,omitempty"`

	Install struct {
		Conflicts     []string            `yaml:"conflicts,omitempty"`
		Extrepo       []string            `yaml:"extrepo,omitempty"`
		Backports     []string            `yaml:"backports,omitempty"`
		BackportSuite string              `yaml:"backport_suite,omitempty"`
		Packages      []string            `yaml:"packages,omitempty"`
		Variants      map[string][]string `yaml:"variants,omitempty"`
		Configs       map[string]string   `yaml:"configs,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Packages []string          `yaml:"packages,omitempty"`
		Configs  map[string]string `yaml:"configs,omitempty"`
	} `yaml:"remove,omitempty"`

	PreInstall  string `yaml:"preinstall,omitempty"`
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
		Name:          name,
		Description:   def.Description,
		Categories:    def.Categories,
		Type:          pkg.TypeApt,
		Depends:       def.Depends,
		Packages:      def.Install.Packages,
		Remove:        def.Remove.Packages,
		Configs:       def.Install.Configs,
		RemoveConfigs: def.Remove.Configs,
		PreInstall:    def.PreInstall,
		PostInstall:   def.PostInstall,
		PostRemove:    def.PostRemove,
		Apt: &pkg.AptConfig{
			Extrepo:       def.Install.Extrepo,
			Backports:     def.Install.Backports,
			BackportSuite: def.Install.BackportSuite,
			Variants:      def.Install.Variants,
			Conflicts:     def.Install.Conflicts,
		},
	}, nil
}
