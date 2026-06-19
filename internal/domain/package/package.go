package pkg

type Type string

const (
	TypeApt    Type = "apt"
	TypeDeb    Type = "deb"
	TypeSource Type = "source"
	TypeConfig Type = "config"
	TypeCore   Type = "core"
)

type Package struct {
	Metadata       `yaml:",inline"`
	RepositorySpec `yaml:",inline"`
	InstallSpec    `yaml:",inline"`
	ConfigSpec     `yaml:",inline"`
}

func (p *Package) Clone() *Package {
	cp := *p
	if p.Variants != nil {
		cp.Variants = make(map[string]string, len(p.Variants))
		for k, v := range p.Variants {
			cp.Variants[k] = v
		}
	}
	if p.Configs != nil {
		cp.Configs = make(map[string]string, len(p.Configs))
		for k, v := range p.Configs {
			cp.Configs[k] = v
		}
	}
	if p.UserConfigs != nil {
		cp.UserConfigs = make(map[string]string, len(p.UserConfigs))
		for k, v := range p.UserConfigs {
			cp.UserConfigs[k] = v
		}
	}
	if p.Depends != nil {
		cp.Depends = make([]string, len(p.Depends))
		copy(cp.Depends, p.Depends)
	}
	if p.Conflicts != nil {
		cp.Conflicts = make([]string, len(p.Conflicts))
		copy(cp.Conflicts, p.Conflicts)
	}
	if p.Packages != nil {
		cp.Packages = make([]string, len(p.Packages))
		copy(cp.Packages, p.Packages)
	}
	if p.Backports != nil {
		cp.Backports = make([]string, len(p.Backports))
		copy(cp.Backports, p.Backports)
	}
	if p.Checks != nil {
		cp.Checks = make([]string, len(p.Checks))
		copy(cp.Checks, p.Checks)
	}
	return &cp
}
