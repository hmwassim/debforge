package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type sourceDefinition struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Depends []string `yaml:"depends,omitempty"`

	Install struct {
		Repo         string   `yaml:"repo,omitempty"`
		SourceSubdir string   `yaml:"source_subdir,omitempty"`
		SkipClone    bool     `yaml:"skip_clone,omitempty"`
		Checks       []string `yaml:"checks,omitempty"`
		VersionCmd   string   `yaml:"version_cmd,omitempty"`
		Packages     []string `yaml:"packages,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Packages []string `yaml:"packages,omitempty"`
	} `yaml:"remove,omitempty"`
}

func parseSource(name string, data []byte) (*pkg.Package, error) {
	var def sourceDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse source definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:         name,
		Type:         pkg.TypeSource,
		Depends:      def.Depends,
		Repo:         def.Install.Repo,
		SourceSubdir: def.Install.SourceSubdir,
		SkipClone:    def.Install.SkipClone,
		Checks:       def.Install.Checks,
		VersionCmd:   def.Install.VersionCmd,
		Packages:     def.Install.Packages,
		Remove:       def.Remove.Packages,
	}, nil
}
