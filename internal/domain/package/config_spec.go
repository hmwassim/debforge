package pkg

type ConfigSpec struct {
	Configs     map[string]string `yaml:"configs,omitempty"`
	UserConfigs map[string]string `yaml:"user_configs,omitempty"`
	ConfigDir   string            `yaml:"-"`
}
