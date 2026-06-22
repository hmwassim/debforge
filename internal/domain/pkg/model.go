package pkg

import "github.com/hmwassim/debforge/internal/registry"

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
	Extrepo   []string
	Packages  []string
	Remove    []string
	Primary   string
	Backports []string
	Variants  map[string]string
	Variant   string

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
	Configs      map[string]string
	RemoveConfigs map[string]string
	UserConfigs  map[string]string

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
	cp.Remove = copySlice(p.Remove)
	cp.Backports = copySlice(p.Backports)
	cp.Extrepo = copySlice(p.Extrepo)
	cp.Variants = copyMap(p.Variants)
	cp.Checks = copySlice(p.Checks)
	cp.Configs = copyMap(p.Configs)
	cp.RemoveConfigs = copyMap(p.RemoveConfigs)
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

// Registry indexes packages by name. It is a thin, name-aware wrapper
// around the shared generic registry.Registry rather than a hand-rolled
// map, so package lookup and the installer lookup in the installer
// package stay implemented identically.
type Registry struct {
	*registry.Registry[string, *Package]
}

func NewRegistry() *Registry {
	return &Registry{Registry: registry.New[string, *Package]()}
}

// Register indexes p under its own name.
func (r *Registry) Register(p *Package) {
	r.Registry.Register(p.Name, p)
}
