package dependency

import (
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/package"
)

type Resolver struct {
	registry *pkg.Registry
}

func NewResolver(registry *pkg.Registry) *Resolver {
	return &Resolver{registry: registry}
}

func (r *Resolver) Resolve(target *pkg.Package, installed map[string]bool, force bool, variants map[string]string) ([]*pkg.Package, error) {
	var ordered []*pkg.Package
	visiting := map[string]bool{}
	added := map[string]bool{}

	if err := r.visit(target, installed, force, variants, visiting, added, &ordered); err != nil {
		return nil, err
	}
	return ordered, nil
}

func (r *Resolver) visit(current *pkg.Package, installed map[string]bool, force bool, variants map[string]string, visiting map[string]bool, added map[string]bool, ordered *[]*pkg.Package) error {
	if added[current.Name] {
		return nil
	}
	if visiting[current.Name] {
		return fmt.Errorf("dependency cycle detected: %s", current.Name)
	}
	visiting[current.Name] = true

	for _, depName := range current.Depends {
		depPkg, ok := r.registry.Lookup(depName)
		if !ok {
			return fmt.Errorf("unknown dependency: %s", depName)
		}
		if installed[depName] && !force {
			continue
		}
		if variant, ok := variants[depName]; ok && len(depPkg.Variants) > 0 {
			depPkg = depPkg.Clone()
			depPkg.Variants = map[string]string{variant: depPkg.Variants[variant]}
		}
		if err := r.visit(depPkg, installed, force, variants, visiting, added, ordered); err != nil {
			return err
		}
	}

	delete(visiting, current.Name)

	if !installed[current.Name] || force {
		*ordered = append(*ordered, current)
		added[current.Name] = true
	}

	return nil
}
