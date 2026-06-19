package installers

import "github.com/hmwassim/debforge/internal/domain/package"

type Registry struct {
	entries map[pkg.Type]Installer
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[pkg.Type]Installer),
	}
}

func (r *Registry) Register(t pkg.Type, inst Installer) {
	r.entries[t] = inst
}

func (r *Registry) Lookup(t pkg.Type) (Installer, bool) {
	inst, ok := r.entries[t]
	return inst, ok
}
