package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type sourceDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Category    string   `yaml:"category,omitempty"`
	Type        string   `yaml:"type"`
	Depends     []string `yaml:"depends,omitempty"`

	Install struct {
		Repo         string   `yaml:"repo,omitempty"`
		URL          string   `yaml:"url,omitempty"`
		SHA256       string   `yaml:"sha256,omitempty"`
		SourceSubdir string   `yaml:"source_subdir,omitempty"`
		SkipClone    bool     `yaml:"skip_clone,omitempty"`
		TagPrefix    string   `yaml:"tag_prefix,omitempty"`
		VersionCmd   string   `yaml:"version_cmd,omitempty"`
		Packages     []string `yaml:"packages,omitempty"`
		Build        string   `yaml:"build,omitempty"`
		Install      string   `yaml:"install,omitempty"`
		Postinstall  string   `yaml:"postinstall,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Script   string   `yaml:"script,omitempty"`
		Packages []string `yaml:"packages,omitempty"`
	} `yaml:"remove,omitempty"`
}

func parseSource(name string, data []byte) (*pkg.Package, error) {
	var def sourceDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse source definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:        name,
		Description: def.Description,
		Category:    def.Category,
		Type:        pkg.TypeSource,
		Depends:     def.Depends,
		Repo:        def.Install.Repo,
		URL:         def.Install.URL,
		SHA256:      def.Install.SHA256,
		TagPrefix:   def.Install.TagPrefix,
		VersionCmd:  def.Install.VersionCmd,
		Packages:    def.Install.Packages,
		Remove:      def.Remove.Packages,
		Source: &pkg.SourceConfig{
			SourceSubdir:      def.Install.SourceSubdir,
			SkipClone:         def.Install.SkipClone,
			BuildScript:       def.Install.Build,
			InstallScript:     def.Install.Install,
			PostinstallScript: def.Install.Postinstall,
			RemoveScript:      def.Remove.Script,
		},
	}, nil
}
