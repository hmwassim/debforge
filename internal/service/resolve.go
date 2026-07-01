package service

import (
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

// Resolver computes the transitive dependency closure for a package in
// topological order (dependencies before dependents).
type Resolver struct {
	reg *pkg.Registry
}

// NewResolver returns a new Resolver.
func NewResolver(reg *pkg.Registry) *Resolver {
	return &Resolver{reg: reg}
}

// Resolve performs a DFS of root's Depends and returns every transitive
// dependency (including root) in topological order (deps before dependents).
// Each returned package is a clone so callers can safely mutate fields.
// Returns an error when a dependency cycle is detected.
func (r *Resolver) Resolve(root *pkg.Package) ([]*pkg.Package, error) {
	seen := map[string]bool{}
	ordered := []*pkg.Package{}
	var dfs func(path []string, name string) error
	dfs = func(path []string, name string) error {
		// Cycle detection must come before the seen/done check: a node
		// that is on the current DFS path but already marked seen from a
		// prior traversal is still a cycle in this call stack.
		for _, p := range path {
			if p == name {
				return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(path, " -> "), name)
			}
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		dep, err := LookupPackage(r.reg, name)
		if err != nil {
			return err
		}
		for _, d := range dep.Depends {
			if err := dfs(append(path, name), d); err != nil {
				return err
			}
		}
		ordered = append(ordered, dep.Clone())
		return nil
	}
	for _, d := range root.Depends {
		if err := dfs([]string{root.Name}, d); err != nil {
			return nil, err
		}
	}
	rootClone := root.Clone()
	ordered = append(ordered, rootClone)
	return ordered, nil
}
