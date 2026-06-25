package definition

import (
	"testing"
)

func TestParseApt_setsPrimary(t *testing.T) {
	data := []byte(`
name: gaming-meta
type: apt
install:
  packages:
    - fizmo-sdl2
    - scummvm
    - gamemode
`)
	p, err := parseApt("gaming-meta", data)
	if err != nil {
		t.Fatalf("parseApt: %v", err)
	}
	if len(p.Packages) == 0 || p.Packages[0] != "fizmo-sdl2" {
		t.Errorf("Packages[0] = %v, want fizmo-sdl2", p.Packages)
	}
}

func TestParseApt_noPrimaryWithoutPackages(t *testing.T) {
	data := []byte(`
name: custom-wine
type: apt
install:
  variants:
    stable: wine-stable
    staging: wine-staging
`)
	p, err := parseApt("custom-wine", data)
	if err != nil {
		t.Fatalf("parseApt: %v", err)
	}
	if len(p.Packages) != 0 {
		t.Errorf("Packages should be empty for variant-only packages, got %v", p.Packages)
	}
}
