package definition

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type appimageDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Category    string   `yaml:"category,omitempty"`
	Type        string   `yaml:"type"`
	SkipUpdate  bool     `yaml:"skip_update,omitempty"`
	Depends     []string `yaml:"depends,omitempty"`
	Repo        string   `yaml:"repo,omitempty"`
	VersionCmd  string   `yaml:"version_cmd,omitempty"`
	TagPrefix   string   `yaml:"tag_prefix,omitempty"`

	Install struct {
		URL      MultiString   `yaml:"url,omitempty"`
		SHA256   MultiString   `yaml:"sha256,omitempty"`
		Packages []string      `yaml:"packages,omitempty"`
		Bin      string        `yaml:"bin,omitempty"`
		Desktop  *desktopEntry `yaml:"desktop,omitempty"`
	} `yaml:"install"`

	Remove struct {
		Packages []string `yaml:"packages,omitempty"`
	} `yaml:"remove,omitempty"`

	PostInstall string `yaml:"post_install,omitempty"`
	PostRemove  string `yaml:"post_remove,omitempty"`
}

type desktopEntry struct {
	ID         string `yaml:"id,omitempty"`
	Name       string `yaml:"name,omitempty"`
	Comment    string `yaml:"comment,omitempty"`
	Icon       string `yaml:"icon,omitempty"`
	Categories string `yaml:"categories,omitempty"`
	Terminal   bool   `yaml:"terminal,omitempty"`
}

func parseAppImage(name string, data []byte) (*pkg.Package, error) {
	var def appimageDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse appimage definition %s: %w", name, err)
	}

	bin := def.Install.Bin
	if bin == "" {
		bin = name
	}

	cfg := &pkg.AppImageConfig{Bin: bin}
	if def.Install.Desktop != nil {
		d := def.Install.Desktop
		desktopName := d.Name
		if desktopName == "" {
			desktopName = name
		}
		categories := d.Categories
		if categories == "" {
			categories = "Utility;"
		}
		cfg.Desktop = &pkg.DesktopEntry{
			ID:         d.ID,
			Name:       desktopName,
			Comment:    d.Comment,
			Icon:       d.Icon,
			Categories: categories,
			Terminal:   d.Terminal,
		}
	}

	return &pkg.Package{
		Name:        name,
		Description: def.Description,
		Category:    def.Category,
		Type:        pkg.TypeAppImage,
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
		AppImg:      cfg,
	}, nil
}
