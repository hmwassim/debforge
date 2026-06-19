package coresetup

import (
	"testing"
)

func TestNewGroups(t *testing.T) {
	g := NewGroups()
	if g == nil {
		t.Fatal("expected non-nil groups")
	}
}

func TestGroupsList(t *testing.T) {
	g := NewGroups()
	list := g.List()
	if len(list) == 0 {
		t.Fatal("expected at least one group")
	}
	for _, gr := range list {
		if gr.Name == "" {
			t.Fatal("group name should not be empty")
		}
	}
}

func TestGroupsLookup(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("system-base")
	if !ok {
		t.Fatal("expected system-base to be found")
	}
	if gr.Name != "system-base" {
		t.Fatalf("expected system-base, got %s", gr.Name)
	}

	_, ok = g.Lookup("nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent")
	}
}

func TestGroupDefsHavePackages(t *testing.T) {
	for _, gr := range GroupDefs {
		if len(gr.Packages) == 0 {
			t.Fatalf("group %s has no packages", gr.Name)
		}
	}
}

func TestSystemBaseHasBackport(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("kernel")
	if !ok {
		t.Fatal("kernel group not found")
	}
	if !gr.Backport {
		t.Fatal("expected kernel group to have Backport=true")
	}
}

func TestSystemFontsHasPostInstall(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("system-fonts")
	if !ok {
		t.Fatal("system-fonts group not found")
	}
	if gr.PostInstall != "fonts" {
		t.Fatalf("expected PostInstall=fonts, got %s", gr.PostInstall)
	}
}

func TestSystemServicesHasConfigs(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("system-services")
	if !ok {
		t.Fatal("system-services group not found")
	}
	if len(gr.Configs) == 0 {
		t.Fatal("expected configs for system-services")
	}
}

func TestMultimediaHasAudioConfig(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("multimedia")
	if !ok {
		t.Fatal("multimedia group not found")
	}
	if len(gr.Configs) == 0 {
		t.Fatal("expected configs for multimedia")
	}
	found := false
	for _, cf := range gr.Configs {
		if cf.Dest == "/etc/security/limits.d/20-audio.conf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected audio limits config")
	}
}

func TestFlatpakHasFlathubPostInstall(t *testing.T) {
	g := NewGroups()
	gr, ok := g.Lookup("flatpak")
	if !ok {
		t.Fatal("flatpak group not found")
	}
	if gr.PostInstall != "flathub" {
		t.Fatalf("expected PostInstall=flathub, got %s", gr.PostInstall)
	}
}
