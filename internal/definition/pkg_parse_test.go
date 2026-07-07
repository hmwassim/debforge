package definition

import (
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func TestParseApt_noPackagesNoVariants(t *testing.T) {
	_, err := parseApt("empty-apt", []byte(`
name: empty-apt
type: apt
install: {}
`))
	if err == nil {
		t.Fatal("expected error for apt without packages or variants")
	}
}

func TestParseApt_badYAML(t *testing.T) {
	_, err := parseApt("bad", []byte(`{{{`))
	if err == nil {
		t.Fatal("expected YAML unmarshal error")
	}
}

func TestParseApt_full(t *testing.T) {
	data := []byte(`
name: gaming-meta
type: apt
description: Gaming packages
depends:
  - base-system
install:
  conflicts:
    - old-package
  extrepo:
    - debian-multimedia
  backports:
    - mesa
  backport_suite: bookworm-backports
  packages:
    - steam
    - lutris
  variants:
    minimal: [steam-installer]
    full: [steam, lutris, gamehub]
  configs:
    /etc/gaming.conf: gaming.conf
remove:
  packages:
    - lutris
  configs:
    /etc/gaming.conf: ""
post_install: echo installed
post_remove: echo removed
`)
	p, err := parseApt("gaming-meta", data)
	if err != nil {
		t.Fatalf("parseApt: %v", err)
	}
	if p.Name != "gaming-meta" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Description != "Gaming packages" {
		t.Errorf("Description = %q", p.Description)
	}
	if p.Type != pkg.TypeApt {
		t.Errorf("Type = %q", p.Type)
	}
	if len(p.Depends) != 1 || p.Depends[0] != "base-system" {
		t.Errorf("Depends = %v", p.Depends)
	}
	if len(p.Packages) != 2 || p.Packages[0] != "steam" {
		t.Errorf("Packages = %v", p.Packages)
	}
	if len(p.Remove) != 1 || p.Remove[0] != "lutris" {
		t.Errorf("Remove = %v", p.Remove)
	}
	if p.Configs["/etc/gaming.conf"] != "gaming.conf" {
		t.Errorf("Configs = %v", p.Configs)
	}
	if p.RemoveConfigs["/etc/gaming.conf"] != "" {
		t.Errorf("RemoveConfigs = %v", p.RemoveConfigs)
	}
	if p.PostInstall != "echo installed" {
		t.Errorf("PostInstall = %q", p.PostInstall)
	}
	if p.PostRemove != "echo removed" {
		t.Errorf("PostRemove = %q", p.PostRemove)
	}
	if p.Apt == nil {
		t.Fatal("Apt is nil")
	}
	if len(p.Apt.Conflicts) != 1 || p.Apt.Conflicts[0] != "old-package" {
		t.Errorf("Conflicts = %v", p.Apt.Conflicts)
	}
	if len(p.Apt.Extrepo) != 1 || p.Apt.Extrepo[0] != "debian-multimedia" {
		t.Errorf("Extrepo = %v", p.Apt.Extrepo)
	}
	if len(p.Apt.Backports) != 1 || p.Apt.Backports[0] != "mesa" {
		t.Errorf("Backports = %v", p.Apt.Backports)
	}
	if p.Apt.BackportSuite != "bookworm-backports" {
		t.Errorf("BackportSuite = %q", p.Apt.BackportSuite)
	}
}

func TestParseDeb_minimal(t *testing.T) {
	p, err := parseDeb("my-deb", []byte(`
name: my-deb
type: deb
package: my-deb-pkg
install:
  url: https://example.com/pkg.deb
`))
	if err != nil {
		t.Fatalf("parseDeb: %v", err)
	}
	if p.Name != "my-deb" || p.Type != pkg.TypeDeb {
		t.Errorf("Name/Type: %+v", p)
	}
	if len(p.URLs) != 1 || p.URLs[0] != "https://example.com/pkg.deb" {
		t.Errorf("URLs = %v", p.URLs)
	}
	if p.Deb == nil || p.Deb.Package != "my-deb-pkg" {
		t.Errorf("Deb = %+v", p.Deb)
	}
}

func TestParseDeb_full(t *testing.T) {
	data := []byte(`
name: my-deb
type: deb
description: A test deb package
package: my-deb-pkg
depends:
  - dep-a
  - dep-b
repo: https://github.com/example/repo
version_cmd: dpkg-query -W -f='${Version}' my-deb-pkg
tag_prefix: v
install:
  url: https://example.com/pkg.deb
  sha256: abc123def456
  packages:
    - extra-pkg
remove:
  packages:
    - extra-pkg
post_install: systemctl start my-service
post_remove: systemctl stop my-service
`)
	p, err := parseDeb("my-deb", data)
	if err != nil {
		t.Fatalf("parseDeb: %v", err)
	}
	if p.Name != "my-deb" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Description != "A test deb package" {
		t.Errorf("Description = %q", p.Description)
	}
	if p.Type != pkg.TypeDeb {
		t.Errorf("Type = %q", p.Type)
	}
	if len(p.Depends) != 2 || p.Depends[1] != "dep-b" {
		t.Errorf("Depends = %v", p.Depends)
	}
	if p.Repo != "https://github.com/example/repo" {
		t.Errorf("Repo = %q", p.Repo)
	}
	if p.VersionCmd != "dpkg-query -W -f='${Version}' my-deb-pkg" {
		t.Errorf("VersionCmd = %q", p.VersionCmd)
	}
	if p.TagPrefix != "v" {
		t.Errorf("TagPrefix = %q", p.TagPrefix)
	}
	if len(p.URLs) != 1 || p.URLs[0] != "https://example.com/pkg.deb" {
		t.Errorf("URLs = %v", p.URLs)
	}
	if len(p.SHA256s) != 1 || p.SHA256s[0] != "abc123def456" {
		t.Errorf("SHA256s = %v", p.SHA256s)
	}
	if len(p.Packages) != 1 || p.Packages[0] != "extra-pkg" {
		t.Errorf("Packages = %v", p.Packages)
	}
	if len(p.Remove) != 1 || p.Remove[0] != "extra-pkg" {
		t.Errorf("Remove = %v", p.Remove)
	}
	if p.PostInstall != "systemctl start my-service" {
		t.Errorf("PostInstall = %q", p.PostInstall)
	}
	if p.PostRemove != "systemctl stop my-service" {
		t.Errorf("PostRemove = %q", p.PostRemove)
	}
	if p.Deb == nil || p.Deb.Package != "my-deb-pkg" {
		t.Errorf("Deb = %+v", p.Deb)
	}
}

func TestParseDeb_badYAML(t *testing.T) {
	_, err := parseDeb("bad", []byte(`{{{`))
	if err == nil {
		t.Fatal("expected YAML unmarshal error")
	}
}

func TestParseSource_minimal(t *testing.T) {
	p, err := parseSource("my-source", []byte(`
name: my-source
type: source
install:
  repo: https://github.com/example/repo
`))
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if p.Name != "my-source" || p.Type != pkg.TypeSource {
		t.Errorf("Name/Type: %+v", p)
	}
	if p.Repo != "https://github.com/example/repo" {
		t.Errorf("Repo = %q", p.Repo)
	}
	if p.Source == nil {
		t.Fatal("Source is nil")
	}
}

func TestParseSource_full(t *testing.T) {
	data := []byte(`
name: my-source
type: source
description: A test source package
depends:
  - dep-a
install:
  repo: https://github.com/example/repo
  url: https://example.com/source.tar.gz
  sha256: abc123
  source_subdir: src
  skip_clone: true
  tag_prefix: v
  version_cmd: cat VERSION
  packages:
    - built-pkg
  build: make build
  install: make install
  postinstall: systemctl daemon-reload
remove:
  script: make uninstall
  packages:
    - built-pkg
`)
	p, err := parseSource("my-source", data)
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if p.Name != "my-source" || p.Description != "A test source package" {
		t.Errorf("Name/Description: %+v", p)
	}
	if p.Type != pkg.TypeSource {
		t.Errorf("Type = %q", p.Type)
	}
	if len(p.Depends) != 1 || p.Depends[0] != "dep-a" {
		t.Errorf("Depends = %v", p.Depends)
	}
	if p.Repo != "https://github.com/example/repo" {
		t.Errorf("Repo = %q", p.Repo)
	}
	if len(p.URLs) != 1 || p.URLs[0] != "https://example.com/source.tar.gz" {
		t.Errorf("URLs = %v", p.URLs)
	}
	if len(p.SHA256s) != 1 || p.SHA256s[0] != "abc123" {
		t.Errorf("SHA256s = %v", p.SHA256s)
	}
	if p.TagPrefix != "v" {
		t.Errorf("TagPrefix = %q", p.TagPrefix)
	}
	if p.VersionCmd != "cat VERSION" {
		t.Errorf("VersionCmd = %q", p.VersionCmd)
	}
	if len(p.Packages) != 1 || p.Packages[0] != "built-pkg" {
		t.Errorf("Packages = %v", p.Packages)
	}
	if len(p.Remove) != 1 || p.Remove[0] != "built-pkg" {
		t.Errorf("Remove = %v", p.Remove)
	}
	if p.Source == nil {
		t.Fatal("Source is nil")
	}
	if p.Source.SourceSubdir != "src" {
		t.Errorf("SourceSubdir = %q", p.Source.SourceSubdir)
	}
	if !p.Source.SkipClone {
		t.Error("SkipClone should be true")
	}
	if p.Source.BuildScript != "make build" {
		t.Errorf("BuildScript = %q", p.Source.BuildScript)
	}
	if p.Source.InstallScript != "make install" {
		t.Errorf("InstallScript = %q", p.Source.InstallScript)
	}
	if p.Source.PostinstallScript != "systemctl daemon-reload" {
		t.Errorf("PostinstallScript = %q", p.Source.PostinstallScript)
	}
	if p.Source.RemoveScript != "make uninstall" {
		t.Errorf("RemoveScript = %q", p.Source.RemoveScript)
	}
}

func TestParseAptSource_badYAML(t *testing.T) {
	_, err := parseSource("bad", []byte(`{{{`))
	if err == nil {
		t.Fatal("expected YAML unmarshal error")
	}
}
