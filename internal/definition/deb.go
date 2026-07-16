package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type debDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Category    string   `yaml:"category,omitempty"`
	Type        string   `yaml:"type"`
	Package     string   `yaml:"package"`
	SkipUpdate  bool     `yaml:"skip_update,omitempty"`
	Depends     []string `yaml:"depends,omitempty"`
	Repo        string   `yaml:"repo,omitempty"`
	VersionCmd  string   `yaml:"version_cmd,omitempty"`
	TagPrefix   string   `yaml:"tag_prefix,omitempty"`

	Install struct {
		URL      MultiString `yaml:"url,omitempty"`
		SHA256   MultiString `yaml:"sha256,omitempty"`
		Packages []string    `yaml:"packages,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Packages []string `yaml:"packages,omitempty"`
	} `yaml:"remove,omitempty"`

	PostInstall string `yaml:"post_install,omitempty"`
	PostRemove  string `yaml:"post_remove,omitempty"`
}

func parseDeb(name string, data []byte) (*pkg.Package, error) {
	var def debDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse deb definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:        name,
		Description: def.Description,
		Category:    def.Category,
		Type:        pkg.TypeDeb,
		Depends:     def.Depends,
		SkipUpdate:  def.SkipUpdate,
		Repo:        def.Repo,
		VersionCmd:  def.VersionCmd,
		TagPrefix:   def.TagPrefix,
		URLs:        def.Install.URL,
		SHA256s:     def.Install.SHA256,
		Packages:    def.Install.Packages,
		Remove:      def.Remove.Packages,
		PostInstall: def.PostInstall,
		PostRemove:  def.PostRemove,
		Deb: &pkg.DebConfig{
			Package: def.Package,
		},
	}, nil
}
