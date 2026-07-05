package format

import (
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/service"
)

func TestFormatInfoOutput_installed(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "firefox", Description: "Web browser", Type: pkg.TypeApt, Category: "browsers", Apt: &pkg.AptConfig{Extrepo: []string{"mozilla"}, Conflicts: []string{"firefox-esr"}}, Packages: []string{"firefox"}})
	st := &service.State{Packages: map[string]service.PkgEntry{"firefox": {Version: "150.0.1"}}}

	out := FormatInfoOutput(reg, st, "firefox", false)
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

	out := FormatInfoOutput(reg, st, "pfetch", false)
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

	out := FormatInfoOutput(reg, st, "cfg-test", true)
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

	outDefault := FormatInfoOutput(reg, st, "script-pkg", false)
	if !strings.Contains(outDefault, "(1 line)") {
		t.Error("expected line count in default mode")
	}

	outVerbose := FormatInfoOutput(reg, st, "script-pkg", true)
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

	out := FormatInfoOutput(reg, st, "vscodium", false)
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

	out := FormatInfoOutput(reg, st, "nvidia", false)
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
	out := FormatInfoOutput(reg, st, "nonexistent", false)
	if out != "" {
		t.Errorf("expected empty output for unknown package, got %q", out)
	}
}

func TestFormatInfoOutput_unknownVersion(t *testing.T) {
	reg := pkg.NewRegistry()
	reg.Register(&pkg.Package{Name: "no-ver", Type: pkg.TypeApt, Category: "misc", Packages: []string{"pkg"}})
	st := &service.State{Packages: map[string]service.PkgEntry{"no-ver": {}}}

	out := FormatInfoOutput(reg, st, "no-ver", false)
	if !strings.Contains(out, "installed (vunknown)") {
		t.Error("expected 'unknown' when version is empty")
	}
}

func TestPluralS(t *testing.T) {
	if pluralS(1) != "" {
		t.Error("expected '' for 1")
	}
	if pluralS(0) != "s" {
		t.Error("expected 's' for 0")
	}
	if pluralS(2) != "s" {
		t.Error("expected 's' for 2")
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

	out := FormatInfoOutput(reg, st, "src-pkg", false)
	if !strings.Contains(out, "build") || !strings.Contains(out, "install") {
		t.Error("expected source script names")
	}
}
