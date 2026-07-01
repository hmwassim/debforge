package service

import (
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func TestResolve_detectsCycle(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "a",
		Type:    pkg.TypeApt,
		Depends: []string{"b"},
	})
	reg.Register(&pkg.Package{
		Name:    "b",
		Type:    pkg.TypeApt,
		Depends: []string{"a"},
	})

	r := NewResolver(reg)
	a, _ := reg.Lookup("a")
	_, err := r.Resolve(a)
	if err == nil {
		t.Fatal("expected error for dependency cycle a -> b -> a, got nil")
	}
	if !strings.Contains(err.Error(), "dependency cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Errorf("cycle error should name the packages in the cycle, got: %v", err)
	}
}

func TestResolve_detectsTransitiveCycle(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "a",
		Type:    pkg.TypeApt,
		Depends: []string{"b"},
	})
	reg.Register(&pkg.Package{
		Name:    "b",
		Type:    pkg.TypeApt,
		Depends: []string{"c"},
	})
	reg.Register(&pkg.Package{
		Name:    "c",
		Type:    pkg.TypeApt,
		Depends: []string{"a"},
	})

	r := NewResolver(reg)
	a, _ := reg.Lookup("a")
	_, err := r.Resolve(a)
	if err == nil {
		t.Fatal("expected error for dependency cycle a -> b -> c -> a, got nil")
	}
}

func TestResolve_detectsCycleAmongNonRootDeps(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "root",
		Type:    pkg.TypeApt,
		Depends: []string{"a"},
	})
	reg.Register(&pkg.Package{
		Name:    "a",
		Type:    pkg.TypeApt,
		Depends: []string{"b"},
	})
	reg.Register(&pkg.Package{
		Name:    "b",
		Type:    pkg.TypeApt,
		Depends: []string{"a"},
	})

	r := NewResolver(reg)
	root, _ := reg.Lookup("root")
	_, err := r.Resolve(root)
	if err == nil {
		t.Fatal("expected error for dependency cycle a -> b -> a (root not in cycle), got nil")
	}
	if !strings.Contains(err.Error(), "dependency cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestResolve_noCycleForSharedDep(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "a",
		Type:    pkg.TypeApt,
		Depends: []string{"c"},
	})
	reg.Register(&pkg.Package{
		Name:    "b",
		Type:    pkg.TypeApt,
		Depends: []string{"c"},
	})
	reg.Register(&pkg.Package{
		Name: "c",
		Type: pkg.TypeApt,
	})

	r := NewResolver(reg)
	a, _ := reg.Lookup("a")
	ordered, err := r.Resolve(a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 2 {
		t.Errorf("expected 2 packages (c then a), got %d", len(ordered))
	}
}

func TestResolve_topologicalOrder(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:    "a",
		Type:    pkg.TypeApt,
		Depends: []string{"b", "c"},
	})
	reg.Register(&pkg.Package{
		Name:    "b",
		Type:    pkg.TypeApt,
		Depends: []string{"d"},
	})
	reg.Register(&pkg.Package{
		Name: "c",
		Type: pkg.TypeApt,
	})
	reg.Register(&pkg.Package{
		Name: "d",
		Type: pkg.TypeApt,
	})

	r := NewResolver(reg)
	a, _ := reg.Lookup("a")
	ordered, err := r.Resolve(a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := make([]string, len(ordered))
	for i, p := range ordered {
		names[i] = p.Name
	}

	// d must come before b, b,c before a
	dIdx := indexOf(names, "d")
	bIdx := indexOf(names, "b")
	cIdx := indexOf(names, "c")
	aIdx := indexOf(names, "a")

	if dIdx > bIdx {
		t.Errorf("d (index %d) should come before b (index %d)", dIdx, bIdx)
	}
	if bIdx > aIdx || (cIdx > aIdx) {
		t.Errorf("deps should come before a (index %d), got b=%d c=%d", aIdx, bIdx, cIdx)
	}
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
