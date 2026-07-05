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
// packages when a variant is selected.
func removePackages(p *pkg.Package) []string {
	pkgs := p.Packages
	if len(p.Remove) > 0 {
		pkgs = p.Remove
	}
	if p.Apt.Variant != "" {
		if v, ok := p.Apt.Variants[p.Apt.Variant]; ok {
			pkgs = append(pkgs, v...)
		}
	}
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
