package definition

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
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
	if p.URL != "https://example.com/pkg.deb" {
		t.Errorf("URL = %q", p.URL)
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
	if p.URL != "https://example.com/pkg.deb" {
		t.Errorf("URL = %q", p.URL)
	}
	if p.SHA256 != "abc123def456" {
		t.Errorf("SHA256 = %q", p.SHA256)
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
	if p.URL != "https://example.com/source.tar.gz" {
		t.Errorf("URL = %q", p.URL)
	}
	if p.SHA256 != "abc123" {
		t.Errorf("SHA256 = %q", p.SHA256)
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

// Parse public API tests

func TestParse_apt(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/apt/test.yaml"] = []byte(`
name: test-pkg
type: apt
install:
  packages:
    - hello
`)
	p, err := Parse("/repo/packages/apt/test.yaml", fs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "test-pkg" || p.Type != pkg.TypeApt {
		t.Errorf("got Name=%q Type=%q", p.Name, p.Type)
	}
}

func TestParse_deb(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/deb/test.yaml"] = []byte(`
name: test-deb
type: deb
package: test-deb-pkg
install:
  url: https://example.com/test.deb
`)
	p, err := Parse("/repo/packages/deb/test.yaml", fs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "test-deb" || p.Type != pkg.TypeDeb {
		t.Errorf("got Name=%q Type=%q", p.Name, p.Type)
	}
}

func TestParse_source(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/source/test.yaml"] = []byte(`
name: test-source
type: source
install:
  repo: https://github.com/example/repo
`)
	p, err := Parse("/repo/packages/source/test.yaml", fs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "test-source" || p.Type != pkg.TypeSource {
		t.Errorf("got Name=%q Type=%q", p.Name, p.Type)
	}
}

func TestParse_config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	yamlPath := "/repo/packages/config/my-app.yaml"
	fs.Files[yamlPath] = []byte(`
name: my-app
type: config
depends:
  - dep-a
install:
  configs:
    /etc/my-app.conf: my-app.conf
  user_configs:
    /home/user/.my-apprc: user.conf
remove:
  configs:
    /etc/my-app.conf: ""
post_install: echo done
post_remove: echo cleanup
`)
	fs.Files["/repo/configs/my-app/my-app.conf"] = []byte("config content")
	fs.Files["/repo/configs/my-app/user.conf"] = []byte("user config content")

	p, err := Parse(yamlPath, fs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Type != pkg.TypeConfig {
		t.Errorf("Type = %q", p.Type)
	}
	if p.Configs["/etc/my-app.conf"] != "config content" {
		t.Errorf("Configs = %v", p.Configs)
	}
	if p.UserConfigs["/home/user/.my-apprc"] != "user config content" {
		t.Errorf("UserConfigs = %v", p.UserConfigs)
	}
	if len(p.Depends) != 1 || p.Depends[0] != "dep-a" {
		t.Errorf("Depends = %v", p.Depends)
	}
	if p.RemoveConfigs["/etc/my-app.conf"] != "" {
		t.Errorf("RemoveConfigs values should be blanked, got %q", p.RemoveConfigs["/etc/my-app.conf"])
	}
	if p.PostInstall != "echo done" || p.PostRemove != "echo cleanup" {
		t.Errorf("scripts: Install=%q Remove=%q", p.PostInstall, p.PostRemove)
	}
}

func TestParse_config_noRemoveConfigs(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	yamlPath := "/repo/packages/config/my-app.yaml"
	fs.Files[yamlPath] = []byte(`
name: my-app
type: config
install:
  configs:
    /etc/my-app.conf: my-app.conf
`)
	fs.Files["/repo/configs/my-app/my-app.conf"] = []byte("content")

	p, err := Parse(yamlPath, fs)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.RemoveConfigs != nil {
		t.Errorf("RemoveConfigs should be nil when not defined, got %v", p.RemoveConfigs)
	}
}

func TestParse_config_missingFile(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/config/my-app.yaml"] = []byte(`
name: my-app
type: config
install:
  configs:
    /etc/my-app.conf: nonexistent.conf
`)
	_, err := Parse("/repo/packages/config/my-app.yaml", fs)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestParse_config_pathTraversal(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/repo/packages/config/my-app.yaml"] = []byte(`
name: my-app
type: config
install:
  configs:
    /etc/shadow: ../../etc/shadow
`)
	_, err := Parse("/repo/packages/config/my-app.yaml", fs)
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

func TestParse_readError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	_, err := Parse("/nonexistent.yaml", fs)
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestParse_badYAML(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/bad.yaml"] = []byte(`{{{`)
	_, err := Parse("/bad.yaml", fs)
	if err == nil {
		t.Fatal("expected YAML error")
	}
}

func TestParse_missingName(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/noname.yaml"] = []byte(`
type: apt
install:
  packages:
    - hello
`)
	_, err := Parse("/noname.yaml", fs)
	if err == nil {
		t.Fatal("expected missing name error")
	}
}

func TestParse_unsupportedType(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/unknown.yaml"] = []byte(`
name: test
type: alien
`)
	_, err := Parse("/unknown.yaml", fs)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestLoadAll_dirNotExist(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	reg := pkg.NewRegistry()
	err := LoadAll("/nonexistent", fs, reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}

func TestLoadAll_loadsYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pkg-a.yaml"), []byte(`
name: pkg-a
type: apt
install:
  packages:
    - hello
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg-b.yaml"), []byte(`
name: pkg-b
type: apt
install:
  packages:
    - world
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("not a yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if _, ok := reg.Lookup("pkg-a"); !ok {
		t.Error("pkg-a not registered")
	}
	if _, ok := reg.Lookup("pkg-b"); !ok {
		t.Error("pkg-b not registered")
	}
}

func TestLoadAll_badYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`{{{`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err == nil {
		t.Fatal("expected error from bad YAML")
	}
}

type walkErrorFS struct {
	ports.FileSystem
}

func (walkErrorFS) Walk(_ string, _ func(string, ports.FileInfo, error) error) error {
	return errors.New("walk failed")
}

func TestLoadAll_walkError(t *testing.T) {
	base := testutil.NewMockFileSystem()
	base.Files["/mydir"] = nil
	base.ExistsFunc = func(_ string) (bool, error) { return true, nil }
	fsys := walkErrorFS{FileSystem: base}
	reg := pkg.NewRegistry()
	err := LoadAll("/mydir", fsys, reg)
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func TestLoadAll_dirExistsButEmpty(t *testing.T) {
	dir := t.TempDir()
	reg := pkg.NewRegistry()
	err := LoadAll(dir, fs.NewFileSystem(), reg)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
}
