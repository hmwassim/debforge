package pkg

import "sync"

type Registry struct {
	mu   sync.RWMutex
	pkgs map[string]*Package
}

func NewRegistry() *Registry {
	return &Registry{
		pkgs: make(map[string]*Package),
	}
}

func (r *Registry) Register(pkg *Package) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pkgs[pkg.Name] = pkg
}

func (r *Registry) Lookup(name string) (*Package, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pkgs[name]
	return p, ok
}

func (r *Registry) List() []*Package {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Package, 0, len(r.pkgs))
	for _, p := range r.pkgs {
		result = append(result, p)
	}
	return result
}

func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pkgs)
}
