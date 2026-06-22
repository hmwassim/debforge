package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type debDefinition struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Depends []string `yaml:"depends,omitempty"`

	Install struct {
		URL           string `yaml:"url,omitempty"`
		Package       string `yaml:"package,omitempty"`
		VersionPrefix string `yaml:"version_prefix,omitempty"`
		AssetMatch    string `yaml:"asset_match,omitempty"`
		AssetArch     string `yaml:"asset_arch,omitempty"`
		SHA256        string `yaml:"sha256,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Package string `yaml:"package,omitempty"`
	} `yaml:"remove,omitempty"`
}

func parseDeb(name string, data []byte) (*pkg.Package, error) {
	var def debDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse deb definition %s: %w", name, err)
	}

	return &pkg.Package{
		Name:          name,
		Type:          pkg.TypeDeb,
		Depends:       def.Depends,
		URL:           def.Install.URL,
		Package:       def.Install.Package,
		VersionPrefix: def.Install.VersionPrefix,
		AssetMatch:    def.Install.AssetMatch,
		AssetArch:     def.Install.AssetArch,
		SHA256:        def.Install.SHA256,
	}, nil
}
