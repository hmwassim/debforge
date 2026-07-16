// Package pkg defines the Package model and package type constants.
package pkg

import (
	"sort"
	"sync"

	"github.com/hmwassim/debforge/internal/registry"
)

// Type identifies the kind of package (apt, deb, source, or config).
type Type string

const (
	TypeApt    Type = "apt"
	TypeDeb    Type = "deb"
	TypeSource Type = "source"
	TypeConfig Type = "config"
)

// AptConfig holds configuration specific to apt-type packages.
type AptConfig struct {
	Extrepo       []string
	Backports     []string
	BackportSuite string
	Variants      map[string][]string
	Variant       string
	Conflicts     []string
}

// DebConfig holds configuration specific to deb-type packages.
type DebConfig struct {
	Package string
}

// SourceConfig holds configuration specific to source-type packages.
type SourceConfig struct {
	SkipClone         bool
	BuildScript       string
	InstallScript     string
	PostinstallScript string
	RemoveScript      string
	SourceSubdir      string
}

// Package represents a single package definition loaded from a YAML file.
type Package struct {
	Name        string
	Description string
	Type        Type
	Depends     []string
	Category    string

	// cross-type
	Packages   []string
	Remove     []string
	URLs       []string
	SHA256s    []string
	SkipUpdate bool

	VersionCmd string
	TagPrefix  string
	Repo       string

	// scripts
	PreInstall  string
	PostInstall string
	PostRemove  string

	// config
	Configs       map[string]string
	RemoveConfigs map[string]string
	UserConfigs   map[string]string

	// runtime flags (not persisted)
	ForceInstall  bool
	SkipRepoSetup bool
	Version       string

	// per-file config hashes (persisted via PkgEntry.ConfigHashes)
	ConfigHashes map[string]string

	// type-specific configuration
	Apt    *AptConfig
	Deb    *DebConfig
	Source *SourceConfig
}

// PrimarySystemPackage returns the primary system package name, preferring
// Deb.Package over Packages[0] over p.Name.
func (p *Package) PrimarySystemPackage() string {
	if p.Deb != nil && p.Deb.Package != "" {
		return p.Deb.Package
	}
	if len(p.Packages) > 0 {
		return p.Packages[0]
	}
	return p.Name
}

// Clone returns a deep copy of p, including all slices and sub-configs.
func (p *Package) Clone() *Package {
	cp := *p
	cp.Depends = copySlice(p.Depends)
	cp.Category = p.Category
	cp.Packages = copySlice(p.Packages)
	cp.Remove = copySlice(p.Remove)
	cp.URLs = copySlice(p.URLs)
	cp.SHA256s = copySlice(p.SHA256s)
	cp.Configs = copyMap(p.Configs)
	cp.RemoveConfigs = copyMap(p.RemoveConfigs)
	cp.UserConfigs = copyMap(p.UserConfigs)
	cp.ConfigHashes = copyMap(p.ConfigHashes)
	if p.Apt != nil {
		c := *p.Apt
		c.Extrepo = copySlice(p.Apt.Extrepo)
		c.Backports = copySlice(p.Apt.Backports)
		c.Variants = copyMapSlice(p.Apt.Variants)
		c.Conflicts = copySlice(p.Apt.Conflicts)
		cp.Apt = &c
	}
	if p.Deb != nil {
		c := *p.Deb
		cp.Deb = &c
	}
	if p.Source != nil {
		c := *p.Source
		cp.Source = &c
	}
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

func copyMapSlice(m map[string][]string) map[string][]string {
	if m == nil {
		return nil
	}
	c := make(map[string][]string, len(m))
	for k, v := range m {
		c[k] = copySlice(v)
	}
	return c
}

// Registry indexes packages by name. It is a thin, name-aware wrapper
// around the shared generic registry.Registry rather than a hand-rolled
// map, so package lookup and the installer lookup in the installer
// package stay implemented identically.
type Registry struct {
	*registry.Registry[string, *Package]

	categoriesMu sync.RWMutex
	categories   map[string][]string
}

// NewRegistry returns an empty package registry.
func NewRegistry() *Registry {
	return &Registry{Registry: registry.New[string, *Package]()}
}

// Register indexes p under its own name and invalidates the
// Categories cache.
func (r *Registry) Register(p *Package) {
	r.Registry.Register(p.Name, p)
	r.categoriesMu.Lock()
	r.categories = nil
	r.categoriesMu.Unlock()
}

// Categories returns a map of category name to sorted package names.
// The result is memoized and invalidated on the next call to Register.
func (r *Registry) Categories() map[string][]string {
	r.categoriesMu.RLock()
	if r.categories != nil {
		cached := r.categories
		r.categoriesMu.RUnlock()
		return cached
	}
	r.categoriesMu.RUnlock()

	idx := map[string][]string{}
	r.Range(func(name string, p *Package) bool {
		if p.Category != "" {
			idx[p.Category] = append(idx[p.Category], name)
		}
		return true
	})
	for _, names := range idx {
		sort.Strings(names)
	}

	r.categoriesMu.Lock()
	if r.categories == nil {
		r.categories = idx
	}
	cached := r.categories
	r.categoriesMu.Unlock()
	return cached
}
