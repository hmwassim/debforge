package pkg

import "testing"

func BenchmarkPackage_Clone(b *testing.B) {
	p := &Package{
		Name:        "bench-pkg",
		Description: "A benchmark package",
		Type:        TypeApt,
		Depends:     []string{"dep-a", "dep-b", "dep-c"},
		Category:    "tools",
		Packages:    []string{"pkg-a", "pkg-b"},
		Remove:      []string{"old-pkg"},
		URLs:        []string{"https://example.com/pkg.deb"},
		SHA256s:     []string{"abc123"},
		Configs:     map[string]string{"/etc/a.conf": "content1", "/etc/b.conf": "content2"},
		Apt: &AptConfig{
			Extrepo:       []string{"nonfree"},
			Backports:     []string{"trixie-backports"},
			BackportSuite: "trixie-backports",
			Variants:      map[string][]string{"desktop": {"extra-pkg"}},
			Conflicts:     []string{"conflicting-pkg"},
		},
	}
	var sink *Package
	for b.Loop() {
		sink = p.Clone()
	}
	_ = sink
}

func BenchmarkPackage_PrimarySystemPackage(b *testing.B) {
	p := &Package{
		Name:     "my-pkg",
		Packages: []string{"system-pkg"},
	}
	var sink string
	for b.Loop() {
		sink = p.PrimarySystemPackage()
	}
	_ = sink
}

func BenchmarkRegistry_Register(b *testing.B) {
	r := NewRegistry()
	p := &Package{Name: "bench-pkg", Type: TypeApt}
	for b.Loop() {
		r.Register(p)
	}
}

func BenchmarkRegistry_Lookup(b *testing.B) {
	r := NewRegistry()
	for i := 0; i < 100; i++ {
		r.Register(&Package{Name: "pkg-" + string(rune('a'+i%26)), Type: TypeApt})
	}
	var sink *Package
	for b.Loop() {
		sink, _ = r.Lookup("pkg-a")
	}
	_ = sink
}

func BenchmarkRegistry_Categories(b *testing.B) {
	r := NewRegistry()
	for i := 0; i < 50; i++ {
		cat := "tools"
		if i%3 == 0 {
			cat = "dev"
		} else if i%3 == 1 {
			cat = "media"
		}
		r.Register(&Package{Name: "pkg-" + string(rune('a'+i%26)), Type: TypeApt, Category: cat})
	}
	var sink map[string][]string
	for b.Loop() {
		sink = r.Categories()
	}
	_ = sink
}
