package installer

import "github.com/hmwassim/debforge/internal/domain/pkg"

type Registry struct {
	impls map[pkg.Type]Installer
}

func NewRegistry() *Registry {
	return &Registry{impls: make(map[pkg.Type]Installer)}
}

func (r *Registry) Register(typ pkg.Type, inst Installer) {
	r.impls[typ] = inst
}

func (r *Registry) Lookup(typ pkg.Type) (Installer, bool) {
	inst, ok := r.impls[typ]
	return inst, ok
}
