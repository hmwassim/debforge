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

func (r *Resolver) Resolve(root *pkg.Package, installed map[string]bool, force bool) ([]*pkg.Package, error) {
	seen := map[string]bool{}
	ordered := []*pkg.Package{}
	var dfs func(name string) error
	dfs = func(name string) error {
		if seen[name] {
			return nil
		}
		seen[name] = true
		if installed[name] && !force {
			return nil
		}
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
