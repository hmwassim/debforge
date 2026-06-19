package pkg

import (
	"testing"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	p := &Package{Metadata: Metadata{Name: "firefox"}}
	r.Register(p)

	got, ok := r.Lookup("firefox")
	if !ok {
		t.Fatal("expected to find firefox")
	}
	if got.Name != "firefox" {
		t.Fatalf("expected firefox, got %s", got.Name)
	}
}

func TestRegistryLookupMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Lookup("nonexistent")
	if ok {
		t.Fatal("expected nonexistent to not be found")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Package{Metadata: Metadata{Name: "a"}})
	r.Register(&Package{Metadata: Metadata{Name: "b"}})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(list))
	}
}

func TestRegistryLen(t *testing.T) {
	r := NewRegistry()
	if r.Len() != 0 {
		t.Fatalf("expected empty registry")
	}
	r.Register(&Package{Metadata: Metadata{Name: "a"}})
	if r.Len() != 1 {
		t.Fatalf("expected 1 package")
	}
}

func TestRegistryOverwrite(t *testing.T) {
	r := NewRegistry()
	r.Register(&Package{Metadata: Metadata{Name: "firefox", Group: "old"}})
	r.Register(&Package{Metadata: Metadata{Name: "firefox", Group: "new"}})

	p, _ := r.Lookup("firefox")
	if p.Group != "new" {
		t.Fatalf("expected group 'new', got '%s'", p.Group)
	}
}

func TestPackageClone(t *testing.T) {
	p := &Package{
		Metadata: Metadata{
			Name:  "test",
			Group: "core",
		},
		InstallSpec: InstallSpec{
			Depends:  []string{"a", "b"},
			Variants: map[string]string{"stable": "repo"},
		},
	}
	cp := p.Clone()
	cp.Name = "modified"
	cp.Depends[0] = "x"
	cp.Variants["stable"] = "changed"

	if p.Name != "test" {
		t.Fatal("clone should not share name")
	}
	if p.Depends[0] != "a" {
		t.Fatal("clone should not share depends slice")
	}
	if p.Variants["stable"] != "repo" {
		t.Fatal("clone should not share variants map")
	}
}
