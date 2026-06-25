package service

import (
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

type Resolver struct {
	reg *pkg.Registry
}

func NewResolver(reg *pkg.Registry) *Resolver {
	return &Resolver{reg: reg}
}

// Resolve performs a DFS of root's Depends and returns every transitive
// dependency (including root) in topological order (deps before dependents).
// Each returned package is a clone so callers can safely mutate fields.
func (r *Resolver) Resolve(root *pkg.Package) ([]*pkg.Package, error) {
	seen := map[string]bool{}
	ordered := []*pkg.Package{}
	var dfs func(name string) error
	dfs = func(name string) error {
		if seen[name] {
			return nil
		}
		seen[name] = true
		dep, err := LookupPackage(r.reg, name)
		if err != nil {
			return err
		}
		for _, d := range dep.Depends {
			if err := dfs(d); err != nil {
				return err
			}
		}
		ordered = append(ordered, dep.Clone())
		return nil
	}
	for _, d := range root.Depends {
		if err := dfs(d); err != nil {
			return nil, err
		}
	}
	rootClone := root.Clone()
	ordered = append(ordered, rootClone)
	return ordered, nil
}
