package format

import (
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/service"
)

func TestFormatSearchOutput_withResults(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Description: "Package B", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{"pkg-a": {}}}

	out := FormatSearchOutput(reg, st, nil)
	if !strings.Contains(out, "[*]") || !strings.Contains(out, "[-]") {
		t.Error("expected both installed [*] and uninstalled [-] markers")
	}
	if !strings.Contains(out, "pkg-a") || !strings.Contains(out, "pkg-b") {
		t.Error("expected both package names in output")
	}
	if !strings.Contains(out, "Package A") || !strings.Contains(out, "Package B") {
		t.Error("expected both descriptions in output")
	}
}

func TestFormatSearchOutput_filtered(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia-driver", Description: "NVIDIA GPU driver", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, []string{"nvidia"})
	if !strings.Contains(out, "nvidia-driver") {
		t.Error("expected nvidia-driver in filtered output")
	}
	if strings.Contains(out, "firefox") {
		t.Error("expected firefox to be filtered out")
	}
}

func TestFormatSearchOutput_matchDescription(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "gpu-tools", Description: "Utilities for NVIDIA GPUs", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "cpu-tools", Description: "CPU utilities", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, []string{"nvidia"})
	if !strings.Contains(out, "gpu-tools") {
		t.Error("expected gpu-tools (matches description 'NVIDIA')")
	}
	if strings.Contains(out, "cpu-tools") {
		t.Error("expected cpu-tools to be filtered out")
	}
}

func TestFormatSearchOutput_noResults(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, []string{"nonexistent"})
	if out != "" {
		t.Errorf("expected empty output for no matches, got %q", out)
	}
}

func TestFormatSearchOutput_emptyPatterns(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, nil)
	if !strings.Contains(out, "pkg-a") || !strings.Contains(out, "pkg-b") {
		t.Error("expected all packages when no patterns")
	}
}

func TestFormatSearchOutput_emptyRegistry(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, nil)
	if out != "" {
		t.Errorf("expected empty output with no packages, got %q", out)
	}
}

func TestFormatSearchOutput_caseInsensitive(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "MyPkg", Description: "My custom package", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "other", Description: "something else", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, []string{"mypkg"})
	if !strings.Contains(out, "MyPkg") {
		t.Error("expected case-insensitive match by name")
	}

	out2 := FormatSearchOutput(reg, st, []string{"CUSTOM"})
	if !strings.Contains(out2, "MyPkg") {
		t.Error("expected case-insensitive match by description")
	}
}

func TestFormatSearchOutput_multiplePatternsJoined(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia-driver", Description: "NVIDIA GPU driver", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := FormatSearchOutput(reg, st, []string{"gpu", "driver"})
	if !strings.Contains(out, "nvidia-driver") {
		t.Error("expected nvidia-driver to match 'gpu driver' in description")
	}
	if strings.Contains(out, "firefox") {
		t.Error("expected firefox to be filtered out")
	}
}

func TestFormatListCategories_withCategories(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers", Description: "Firefox"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming", Description: "Lutris"})

	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := FormatListCategories(reg, st)

	if !strings.Contains(out, "gaming") || !strings.Contains(out, "browsers") {
		t.Errorf("expected categories in output, got %q", out)
	}
	if !strings.Contains(out, "(0/2)") || !strings.Contains(out, "(0/1)") {
		t.Errorf("expected counts in output, got %q", out)
	}
	if !strings.Contains(out, "[i]") || !strings.Contains(out, "gaming") {
		t.Errorf("expected marker and categories, got %q", out)
	}
}

func TestFormatListCategories_fullyInstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming", Description: "Lutris"})

	st := &service.State{Packages: map[string]service.PkgEntry{
		"steam":  {},
		"lutris": {},
	}}
	out := FormatListCategories(reg, st)

	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] for fully installed category, got %q", out)
	}
	if !strings.Contains(out, "(2/2)") {
		t.Errorf("expected (2/2) count, got %q", out)
	}
}

func TestFormatListCategories_empty(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := FormatListCategories(reg, st)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestFormatListCategory_existing(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam platform"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming", Description: "Lutris"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers", Description: "Firefox"})

	st := &service.State{Packages: map[string]service.PkgEntry{}}
	st.Packages["steam"] = service.PkgEntry{}

	out := FormatListCategory(reg, st, "gaming")
	if !strings.Contains(out, "steam") || !strings.Contains(out, "lutris") {
		t.Errorf("expected gaming packages, got %q", out)
	}
	if !strings.Contains(out, "[*]") || !strings.Contains(out, "[-]") {
		t.Errorf("expected installed markers, got %q", out)
	}
	if !strings.HasPrefix(out, "gaming") {
		t.Errorf("expected category header, got %q", out)
	}
}

func TestFormatListCategory_nonExisting(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming"})
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := FormatListCategory(reg, st, "nonexistent")
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestFormatListPackages_withCategories(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers", Description: "Firefox"})

	st := &service.State{Packages: map[string]service.PkgEntry{}}
	st.Packages["steam"] = service.PkgEntry{}

	out := FormatListPackages(reg, st)
	if !strings.Contains(out, "gaming") || !strings.Contains(out, "browsers") {
		t.Errorf("expected category headers, got %q", out)
	}
	if !strings.Contains(out, "[*]") || !strings.Contains(out, "steam") {
		t.Errorf("expected installed steam, got %q", out)
	}
	if !strings.Contains(out, "[-]") || !strings.Contains(out, "firefox") {
		t.Errorf("expected available firefox, got %q", out)
	}
}

func TestSortedMapKeys(t *testing.T) {
	m := map[string]string{"z": "1", "a": "2", "m": "3"}
	keys := sortedMapKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "m" || keys[2] != "z" {
		t.Errorf("expected [a m z], got %v", keys)
	}
}

func TestSortedMapKeys_empty(t *testing.T) {
	keys := sortedMapKeys(nil)
	if len(keys) != 0 {
		t.Errorf("expected empty, got %v", keys)
	}
}

func TestFormatListPackages_empty(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := FormatListPackages(reg, st)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}
