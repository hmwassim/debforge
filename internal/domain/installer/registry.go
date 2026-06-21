package installer

import (
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/registry"
)

// Registry maps a package Type to the Installer that handles it. It is an
// alias for the shared generic registry.Registry, so this lookup table and
// the package-name lookup table in the pkg package share one implementation
// instead of two hand-written copies of the same map+Register+Lookup code.
type Registry = registry.Registry[pkg.Type, Installer]

func NewRegistry() *Registry {
	return registry.New[pkg.Type, Installer]()
}
