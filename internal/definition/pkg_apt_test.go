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
	if p.Primary != "fizmo-sdl2" {
		t.Errorf("Primary = %q, want fizmo-sdl2", p.Primary)
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
	if p.Primary != "" {
		t.Errorf("Primary should be empty for variant-only packages, got %q", p.Primary)
	}
}
