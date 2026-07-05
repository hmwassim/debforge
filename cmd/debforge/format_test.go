package main

import (
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/service"
)

// ---- formatSearchOutput tests ----------------------------------------------

func TestFormatSearchOutput_withResults(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Description: "Package A", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Description: "Package B", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{"pkg-a": {}}}

	out := formatSearchOutput(reg, st, nil)
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

	out := formatSearchOutput(reg, st, []string{"nvidia"})
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

	out := formatSearchOutput(reg, st, []string{"nvidia"})
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

	out := formatSearchOutput(reg, st, []string{"nonexistent"})
	if out != "" {
		t.Errorf("expected empty output for no matches, got %q", out)
	}
}

func TestFormatSearchOutput_emptyPatterns(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pkg-a", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "pkg-b", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, nil)
	if !strings.Contains(out, "pkg-a") || !strings.Contains(out, "pkg-b") {
		t.Error("expected all packages when no patterns")
	}
}

func TestFormatSearchOutput_emptyRegistry(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, nil)
	if out != "" {
		t.Errorf("expected empty output with no packages, got %q", out)
	}
}

func TestFormatSearchOutput_caseInsensitive(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "MyPkg", Description: "My custom package", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "other", Description: "something else", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatSearchOutput(reg, st, []string{"mypkg"})
	if !strings.Contains(out, "MyPkg") {
		t.Error("expected case-insensitive match by name")
	}

	out2 := formatSearchOutput(reg, st, []string{"CUSTOM"})
	if !strings.Contains(out2, "MyPkg") {
		t.Error("expected case-insensitive match by description")
	}
}

func TestFormatSearchOutput_multiplePatternsJoined(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "nvidia-driver", Description: "NVIDIA GPU driver", Type: pkg.TypeApt})
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	// patterns are joined with space and matched as a single substring.
	out := formatSearchOutput(reg, st, []string{"gpu", "driver"})
	if !strings.Contains(out, "nvidia-driver") {
		t.Error("expected nvidia-driver to match 'gpu driver' in description")
	}
	if strings.Contains(out, "firefox") {
		t.Error("expected firefox to be filtered out")
	}
}

// ---- list formatting --------------------------------------------------------

func TestFormatListCategories_withCategories(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "steam", Category: "gaming", Description: "Steam"})
	reg.Register(&pkg.Package{Name: "firefox", Category: "browsers", Description: "Firefox"})
	reg.Register(&pkg.Package{Name: "lutris", Category: "gaming", Description: "Lutris"})

	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := formatListCategories(reg, st)

	if !strings.Contains(out, "gaming") || !strings.Contains(out, "browsers") {
		t.Errorf("expected categories in output, got %q", out)
	}
	if !strings.Contains(out, "(2)") || !strings.Contains(out, "(1)") {
		t.Errorf("expected counts in output, got %q", out)
	}
	if !strings.Contains(out, "[i]") || !strings.Contains(out, "gaming") {
		t.Errorf("expected marker and categories, got %q", out)
	}
}

func TestFormatListCategories_empty(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := formatListCategories(reg, st)
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

	out := formatListCategory(reg, st, "gaming")
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
	out := formatListCategory(reg, st, "nonexistent")
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

	out := formatListPackages(reg, st)
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

func TestFormatListPackages_empty(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := formatListPackages(reg, st)
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

// ---- formatInfoOutput tests ------------------------------------------------

func TestFormatInfoOutput_installed(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt, Category: "browsers", Apt: &pkg.AptConfig{Extrepo: []string{"mozilla"}, Conflicts: []string{"firefox-esr"}}, Packages: []string{"firefox"}})
	st := &service.State{Packages: map[string]service.PkgEntry{"firefox": {Version: "150.0.1"}}}

	out := formatInfoOutput(reg, st, "firefox", false)
	if !strings.Contains(out, "[*]") {
		t.Error("expected [*] for installed package")
	}
	if !strings.Contains(out, "firefox") {
		t.Error("expected package name")
	}
	if !strings.Contains(out, "Web browser") {
		t.Error("expected description")
	}
	if !strings.Contains(out, "installed (v150.0.1)") {
		t.Error("expected version info")
	}
	if !strings.Contains(out, "extrepo:") || !strings.Contains(out, "mozilla") {
		t.Error("expected extrepo info")
	}
	if !strings.Contains(out, "conflicts:") || !strings.Contains(out, "firefox-esr") {
		t.Error("expected conflicts info")
	}
}

func TestFormatInfoOutput_uninstalled(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "pfetch", Description: "Pretty fetch", Type: pkg.TypeSource, Category: "utils", Source: &pkg.SourceConfig{InstallScript: "cp pfetch /usr/local/bin\n"}})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatInfoOutput(reg, st, "pfetch", false)
	if !strings.Contains(out, "[-]") {
		t.Error("expected [-] for uninstalled package")
	}
	if !strings.Contains(out, "not installed") {
		t.Error("expected not installed status")
	}
	if !strings.Contains(out, "source") {
		t.Error("expected type section")
	}
}

func TestFormatInfoOutput_verboseConfig(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:        "cfg-test",
		Description: "Config test",
		Type:        pkg.TypeConfig,
		Category:    "config",
		Configs:     map[string]string{"/etc/test.conf": "setting=true\n"},
	})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatInfoOutput(reg, st, "cfg-test", true)
	if !strings.Contains(out, "[i]") {
		t.Error("expected [i] marker")
	}
	if !strings.Contains(out, "setting=true") {
		t.Error("expected config content in verbose mode")
	}
}

func TestFormatInfoOutput_verboseScripts(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name:        "script-pkg",
		Type:        pkg.TypeApt,
		Category:    "dev",
		Packages:    []string{"hello"},
		PostInstall: "echo done\n",
	})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	outDefault := formatInfoOutput(reg, st, "script-pkg", false)
	if !strings.Contains(outDefault, "(1 line)") {
		t.Error("expected line count in default mode")
	}

	outVerbose := formatInfoOutput(reg, st, "script-pkg", true)
	if !strings.Contains(outVerbose, "echo done") {
		t.Error("expected script content in verbose mode")
	}
}

func TestFormatInfoOutput_debType(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "vscodium", Description: "VS Code without MS branding",
		Type: pkg.TypeDeb, Category: "dev",
		Deb: &pkg.DebConfig{Package: "codium"},
		URL: "https://example.com/codium.deb",
	})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatInfoOutput(reg, st, "vscodium", false)
	if !strings.Contains(out, "package:") || !strings.Contains(out, "codium") {
		t.Error("expected deb package field")
	}
	if !strings.Contains(out, "url:") || !strings.Contains(out, "example.com") {
		t.Error("expected url field")
	}
}

func TestFormatInfoOutput_dependsAndRemove(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "nvidia", Description: "NVIDIA drivers",
		Type: pkg.TypeApt, Category: "drivers",
		Depends: []string{"some-dep"},
		Remove:  []string{"nouveau"},
		Apt:     &pkg.AptConfig{Extrepo: []string{"nvidia-cuda"}},
	})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatInfoOutput(reg, st, "nvidia", false)
	if !strings.Contains(out, "depends:") || !strings.Contains(out, "some-dep") {
		t.Error("expected depends info")
	}
	if !strings.Contains(out, "remove") || !strings.Contains(out, "nouveau") {
		t.Error("expected remove section")
	}
}

func TestFormatInfoOutput_unknownPackage(t *testing.T) {
	reg := pkg.NewRegistry()
	st := &service.State{Packages: map[string]service.PkgEntry{}}
	out := formatInfoOutput(reg, st, "nonexistent", false)
	if out != "" {
		t.Errorf("expected empty output for unknown package, got %q", out)
	}
}

func TestFormatInfoOutput_unknownVersion(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "no-ver", Type: pkg.TypeApt, Category: "misc", Packages: []string{"pkg"}})
	st := &service.State{Packages: map[string]service.PkgEntry{"no-ver": {}}}

	out := formatInfoOutput(reg, st, "no-ver", false)
	if !strings.Contains(out, "installed (vunknown)") {
		t.Error("expected 'unknown' when version is empty")
	}
}

func TestFormatInfoOutput_sourceScripts(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{
		Name: "src-pkg", Type: pkg.TypeSource, Category: "utils",
		Source: &pkg.SourceConfig{
			BuildScript:   "make\n",
			InstallScript: "make install\n",
		},
	})
	st := &service.State{Packages: map[string]service.PkgEntry{}}

	out := formatInfoOutput(reg, st, "src-pkg", false)
	if !strings.Contains(out, "build") || !strings.Contains(out, "install") {
		t.Error("expected source script names")
	}
}
