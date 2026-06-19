package pkg

type InstallSpec struct {
	Package       string            `yaml:"package,omitempty"`
	VersionPrefix string            `yaml:"version_prefix,omitempty"`
	AssetMatch    string            `yaml:"asset_match,omitempty"`
	AssetArch     string            `yaml:"asset_arch,omitempty"`
	SourceSubdir  string            `yaml:"source_subdir,omitempty"`
	VersionCmd    string            `yaml:"version_cmd,omitempty"`
	Variants      map[string]string `yaml:"variants,omitempty"`
	Version       string            `yaml:"version,omitempty"`
	Checks        []string          `yaml:"checks,omitempty"`
	SkipClone     bool              `yaml:"skip_clone,omitempty"`
	PostInstall   string            `yaml:"post_install,omitempty"`
	PostRemove    string            `yaml:"post_remove,omitempty"`
	ForceInstall  bool              `yaml:"force_install,omitempty"`
	Interactive   bool              `yaml:"interactive,omitempty"`
	Clean         bool              `yaml:"clean,omitempty"`
	Depends       []string          `yaml:"depends,omitempty"`
	Conflicts     []string          `yaml:"conflicts,omitempty"`
	SHA256        string            `yaml:"sha256,omitempty"`
}
