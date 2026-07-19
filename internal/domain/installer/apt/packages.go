package apt

import "github.com/hmwassim/debforge/internal/domain/pkg"

// installPackages returns the effective list of system packages to install
// for p, including variant packages when a variant is selected.
func installPackages(p *pkg.Package) []string {
	pkgs := append([]string(nil), p.Packages...)
	if p.Apt != nil && p.Apt.Variant != "" {
		if v, ok := p.Apt.Variants[p.Apt.Variant]; ok {
			pkgs = append(pkgs, v...)
		}
	}
	return pkgs
}

// removePackages returns the effective list of system packages to remove
// for p, preferring p.Remove over p.Packages and including variant
// packages when a variant is selected. Returns a new slice so the
// original p.Remove / p.Packages backing array is never mutated.
func removePackages(p *pkg.Package) []string {
	src := p.Packages
	if len(p.Remove) > 0 {
		src = p.Remove
	}
	var variantPkgs []string
	if p.Apt != nil && p.Apt.Variant != "" {
		variantPkgs = p.Apt.Variants[p.Apt.Variant]
	}
	pkgs := make([]string, len(src), len(src)+len(variantPkgs))
	copy(pkgs, src)
	pkgs = append(pkgs, variantPkgs...)
	return pkgs
}

// primarySystemPackage returns the primary system package name for p,
// preferring the first variant package when a variant is selected.
func primarySystemPackage(p *pkg.Package) string {
	if p.Apt != nil && p.Apt.Variant != "" {
		if v, ok := p.Apt.Variants[p.Apt.Variant]; ok && len(v) > 0 {
			return v[0]
		}
	}
	return p.PrimarySystemPackage()
}
