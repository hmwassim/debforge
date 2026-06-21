package pkg

type Type string

const (
	TypeApt    Type = "apt"
	TypeDeb    Type = "deb"
	TypeSource Type = "source"
	TypeConfig Type = "config"
)

type Package struct {
	Name      string
	Type      Type
	Depends   []string
	Conflicts []string

	// apt
	Extrepo   string
	Packages  []string
	Primary   string
	Backports []string

	// deb
	URL           string
	Package       string
	VersionPrefix string
	AssetMatch    string
	AssetArch     string
	SHA256        string

	// source
	Repo         string
	SourceSubdir string
	SkipClone    bool
	Checks       []string
	VersionCmd   string

	// config
	Configs    map[string]string
	UserConfigs map[string]string

	// scripts
	PostInstall string
	PostRemove  string

	// metadata
	ForceInstall bool
	Version      string
}

func (p *Package) Clone() *Package {
	cp := *p
	cp.Depends = copySlice(p.Depends)
	cp.Conflicts = copySlice(p.Conflicts)
	cp.Packages = copySlice(p.Packages)
	cp.Backports = copySlice(p.Backports)
	cp.Checks = copySlice(p.Checks)
	cp.Configs = copyMap(p.Configs)
	cp.UserConfigs = copyMap(p.UserConfigs)
	return &cp
}

func copySlice(s []string) []string {
	if s == nil {
		return nil
	}
	c := make([]string, len(s))
	copy(c, s)
	return c
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

type Registry struct {
	pkgs map[string]*Package
}

func NewRegistry() *Registry {
	return &Registry{pkgs: make(map[string]*Package)}
}

func (r *Registry) Register(p *Package) {
	r.pkgs[p.Name] = p
}

func (r *Registry) Lookup(name string) (*Package, bool) {
	p, ok := r.pkgs[name]
	return p, ok
}
