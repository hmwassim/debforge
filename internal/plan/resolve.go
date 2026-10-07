package plan

import (
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/catalog"
)

// Resolve returns roots and all their transitive dependencies in
// topological order (dependencies first). Cycles and unknown packages are
// errors.
func Resolve(c *catalog.Catalog, roots []string) ([]*catalog.Package, error) {
	const (
		visiting = 1
		done     = 2
	)
	mark := map[string]int{}
	var out []*catalog.Package
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch mark[name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(path, " -> "), name)
		}
		p, ok := c.Get(name)
		if !ok {
			if len(path) > 0 {
				return fmt.Errorf("%s depends on unknown package %q", path[len(path)-1], name)
			}
			return fmt.Errorf("unknown package %q", name)
		}
		mark[name] = visiting
		next := append(append([]string(nil), path...), name)
		for _, d := range p.Depends {
			if err := visit(d, next); err != nil {
				return err
			}
		}
		mark[name] = done
		out = append(out, p)
		return nil
	}
	for _, r := range roots {
		if err := visit(r, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}
