package pkg

type RepositorySpec struct {
	Packages   []string `yaml:"packages,omitempty"`
	Repo       string   `yaml:"repo,omitempty"`
	URL        string   `yaml:"url,omitempty"`
	Extrepo    string   `yaml:"extrepo,omitempty"`
	KeyURL     string   `yaml:"key_url,omitempty"`
	KeyPath    string   `yaml:"key_path,omitempty"`
	KeyDearmor bool     `yaml:"key_dearmor,omitempty"`
	Sources    string   `yaml:"sources,omitempty"`
	SourcePath string   `yaml:"source_path,omitempty"`
	Primary    string   `yaml:"primary,omitempty"`
	Backports  []string `yaml:"backports,omitempty"`
}
