package definition

import (
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestParse_apt(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	fs := testutil.NewMockFileSystem()
	_, err := Parse("/nonexistent.yaml", fs)
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestParse_badYAML(t *testing.T) {
	t.Parallel()
	fs := testutil.NewMockFileSystem()
	fs.Files["/bad.yaml"] = []byte(`{{{`)
	_, err := Parse("/bad.yaml", fs)
	if err == nil {
		t.Fatal("expected YAML error")
	}
}

func TestParse_missingName(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
