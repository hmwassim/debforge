package pkg

type Metadata struct {
	Name        string `yaml:"name"`
	Type        Type   `yaml:"type"`
	Description string `yaml:"description,omitempty"`
	Group       string `yaml:"group,omitempty"`
	Category    string `yaml:"category,omitempty"`
}
