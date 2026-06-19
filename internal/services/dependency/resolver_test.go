package dependency

import (
	"testing"

	"github.com/hmwassim/debforge/internal/domain/package"
)

func TestResolveSimple(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{Metadata: pkg.Metadata{Name: "a"}}
	reg.Register(a)

	r := NewResolver(reg)
	ordered, err := r.Resolve(a, nil, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 1 || ordered[0].Name != "a" {
		t.Fatalf("expected [a], got %v", names(ordered))
	}
}

func TestResolveWithDeps(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"b"}},
	}
	b := &pkg.Package{Metadata: pkg.Metadata{Name: "b"}}
	reg.Register(a)
	reg.Register(b)

	r := NewResolver(reg)
	ordered, err := r.Resolve(a, nil, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 2 || ordered[0].Name != "b" || ordered[1].Name != "a" {
		t.Fatalf("expected [b, a], got %v", names(ordered))
	}
}

func TestResolveCycle(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"b"}},
	}
	b := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "b"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"a"}},
	}
	reg.Register(a)
	reg.Register(b)

	r := NewResolver(reg)
	_, err := r.Resolve(a, nil, false, nil)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestResolveAlreadyInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"b"}},
	}
	b := &pkg.Package{Metadata: pkg.Metadata{Name: "b"}}
	reg.Register(a)
	reg.Register(b)

	r := NewResolver(reg)
	installed := map[string]bool{"b": true}
	ordered, err := r.Resolve(a, installed, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 1 || ordered[0].Name != "a" {
		t.Fatalf("expected [a] (b already installed), got %v", names(ordered))
	}
}

func TestResolveForceReinstall(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"b"}},
	}
	b := &pkg.Package{Metadata: pkg.Metadata{Name: "b"}}
	reg.Register(a)
	reg.Register(b)

	r := NewResolver(reg)
	installed := map[string]bool{"b": true}
	ordered, err := r.Resolve(a, installed, true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 2 {
		t.Fatalf("expected [b, a] (force reinstall), got %v", names(ordered))
	}
}

func TestResolveUnknownDep(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"unknown"}},
	}
	reg.Register(a)

	r := NewResolver(reg)
	_, err := r.Resolve(a, nil, false, nil)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestResolveDeepDeps(t *testing.T) {
	reg := pkg.NewRegistry()
	a := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "a"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"b"}},
	}
	b := &pkg.Package{
		Metadata:    pkg.Metadata{Name: "b"},
		InstallSpec: pkg.InstallSpec{Depends: []string{"c"}},
	}
	c := &pkg.Package{Metadata: pkg.Metadata{Name: "c"}}
	reg.Register(a)
	reg.Register(b)
	reg.Register(c)

	r := NewResolver(reg)
	ordered, err := r.Resolve(a, nil, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 3 || ordered[0].Name != "c" || ordered[1].Name != "b" || ordered[2].Name != "a" {
		t.Fatalf("expected [c, b, a], got %v", names(ordered))
	}
}

func names(pkgs []*pkg.Package) []string {
	var n []string
	for _, p := range pkgs {
		n = append(n, p.Name)
	}
	return n
}
