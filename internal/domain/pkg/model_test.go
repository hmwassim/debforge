package pkg

import "testing"

// ---- PrimarySystemPackage ---------------------------------------------
//
// This is the fix for the apt-idempotency bug: previously the code asked
// dpkg about a single package.Package field that was only ever populated
// for deb-type packages, so apt bundles (e.g. a "gaming-meta" definition
// installing 20 real apt packages under a debforge name that is not
// itself a real dpkg package) could never be recognized as installed.

func TestPrimarySystemPackage_prefersDebPackage(t *testing.T) {
	p := &Package{
		Name:     "my-deb-pkg",
		Packages: []string{"some-other-name"},
		Deb:      &DebConfig{Package: "real-dpkg-name"},
	}
	if got := p.PrimarySystemPackage(); got != "real-dpkg-name" {
		t.Errorf("got %q, want %q", got, "real-dpkg-name")
	}
}

func TestPrimarySystemPackage_fallsBackToFirstPackage(t *testing.T) {
	p := &Package{Name: "wine", Packages: []string{"wine", "wine32", "wine64"}}
	if got := p.PrimarySystemPackage(); got != "wine" {
		t.Errorf("got %q, want %q", got, "wine")
	}
}

func TestPrimarySystemPackage_fallsBackToName(t *testing.T) {
	p := &Package{Name: "firefox"}
	if got := p.PrimarySystemPackage(); got != "firefox" {
		t.Errorf("got %q, want %q", got, "firefox")
	}
}

func TestPrimarySystemPackage_emptyDebPackageFallsThrough(t *testing.T) {
	p := &Package{Name: "x", Packages: []string{"x-real"}, Deb: &DebConfig{Package: ""}}
	if got := p.PrimarySystemPackage(); got != "x-real" {
		t.Errorf("expected empty Deb.Package to fall through to Packages[0], got %q", got)
	}
}

// ---- Clone --------------------------------------------------------------
//
// Clone must be a real deep copy: Resolve() relies on cloning registry
// templates before mutating per-install fields (ForceInstall,
// SkipRepoSetup, Apt.Variant, ...), so a shallow copy here would let one
// package's install mutate every other install's shared template.

func TestClone_mutatingSlicesDoesNotAffectOriginal(t *testing.T) {
	orig := &Package{
		Name:     "p",
		Depends:  []string{"a", "b"},
		Packages: []string{"pkg1"},
		Remove:   []string{"r1"},
	}
	clone := orig.Clone()
	clone.Depends[0] = "MUTATED"
	clone.Packages = append(clone.Packages, "pkg2")
	clone.Remove[0] = "MUTATED"

	if orig.Depends[0] != "a" {
		t.Errorf("mutating clone.Depends affected original: %v", orig.Depends)
	}
	if len(orig.Packages) != 1 {
		t.Errorf("appending to clone.Packages affected original: %v", orig.Packages)
	}
	if orig.Remove[0] != "r1" {
		t.Errorf("mutating clone.Remove affected original: %v", orig.Remove)
	}
}

func TestClone_mutatingMapsDoesNotAffectOriginal(t *testing.T) {
	orig := &Package{
		Name:    "p",
		Configs: map[string]string{"/etc/x.conf": "content"},
	}
	clone := orig.Clone()
	clone.Configs["/etc/x.conf"] = "MUTATED"
	clone.Configs["/etc/new.conf"] = "added"

	if orig.Configs["/etc/x.conf"] != "content" {
		t.Errorf("mutating clone.Configs affected original: %v", orig.Configs)
	}
	if _, ok := orig.Configs["/etc/new.conf"]; ok {
		t.Errorf("adding a key to clone.Configs affected original: %v", orig.Configs)
	}
}

func TestClone_mutatingAptVariantDoesNotAffectOriginal(t *testing.T) {
	orig := &Package{
		Name: "wine",
		Apt: &AptConfig{
			Variants:  map[string][]string{"stable": {"wine-stable"}},
			Conflicts: []string{"playonlinux"},
		},
	}
	clone := orig.Clone()
	clone.Apt.Variant = "stable"
	clone.Apt.Variants["staging"] = []string{"wine-staging"}
	clone.Apt.Conflicts[0] = "MUTATED"

	if orig.Apt.Variant != "" {
		t.Errorf("setting clone.Apt.Variant affected original: %q", orig.Apt.Variant)
	}
	if _, ok := orig.Apt.Variants["staging"]; ok {
		t.Errorf("adding to clone.Apt.Variants affected original: %v", orig.Apt.Variants)
	}
	if orig.Apt.Conflicts[0] != "playonlinux" {
		t.Errorf("mutating clone.Apt.Conflicts affected original: %v", orig.Apt.Conflicts)
	}
}

func TestClone_nilSubConfigsStayNil(t *testing.T) {
	orig := &Package{Name: "p"}
	clone := orig.Clone()
	if clone.Apt != nil || clone.Deb != nil || clone.Source != nil {
		t.Errorf("expected nil sub-configs to remain nil after Clone, got Apt=%v Deb=%v Source=%v", clone.Apt, clone.Deb, clone.Source)
	}
}

func TestClone_runtimeFlagsAreCopiedButIndependentGoingForward(t *testing.T) {
	orig := &Package{Name: "p", ForceInstall: true, SkipRepoSetup: false}
	clone := orig.Clone()
	if !clone.ForceInstall {
		t.Error("expected ForceInstall to be copied onto the clone")
	}
	clone.SkipRepoSetup = true
	if orig.SkipRepoSetup {
		t.Error("mutating clone.SkipRepoSetup affected the original")
	}
}

// ---- Registry -------------------------------------------------------------

func TestPkgRegistry_registerIndexesByName(t *testing.T) {
	r := NewRegistry()
	r.Register(&Package{Name: "firefox"})

	p, ok := r.Lookup("firefox")
	if !ok || p.Name != "firefox" {
		t.Errorf("got (%v, %v)", p, ok)
	}
}
