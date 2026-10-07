package catalog

import (
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hmwassim/debforge/internal/system"
)

func load(t *testing.T, files map[string]string) (*Catalog, []string, error) {
	t.Helper()
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return Load(Layer{Name: "test", FS: m, PkgDir: "packages", FilesDir: "files"})
}

const firefox = `
name: firefox
description: Firefox
category: browsers
source:
  apt:
    packages: [firefox]
    extrepo: [mozilla]
    conflicts: [firefox-esr]
`

func TestLoadValid(t *testing.T) {
	c, _, err := load(t, map[string]string{
		"packages/firefox.yaml": firefox,
		"packages/sys.yaml": `
name: config-system
description: tuning
category: system
depends: [firefox]
files:
  - dest: /etc/sysctl.d/99-x.conf
    src: sysctl.conf
  - dest: ~/.config/x.conf
    content: "a=1\n"
    mode: "0600"
reload: [sysctl]
`,
		"files/config-system/sysctl.conf": "vm.x=1\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Get("config-system")
	if p.Kind() != KindConfig || string(p.Files[0].Data()) != "vm.x=1\n" || p.Files[1].FileMode() != 0o600 || !p.Files[1].User() {
		t.Fatalf("bad parse: %+v", p)
	}
	if f, _ := c.Get("firefox"); f.Kind() != KindApt || f.Hash == "" {
		t.Fatal("firefox kind/hash")
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	// The old winetricks bug: hooks nested in the wrong place were ignored.
	_, _, err := load(t, map[string]string{"packages/w.yaml": `
name: winetricks
description: x
category: gaming
source:
  apt:
    packages: [winetricks]
    postinstall: curl ...
`})
	if err == nil || !strings.Contains(err.Error(), "postinstall") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]string{
		"no sha256": `
name: d
description: x
category: utils
source:
  deb:
    urls: [{url: "https://e.com/d.deb"}]`,
		"http url": `
name: d
description: x
category: utils
source:
  deb:
    urls: [{url: "http://e.com/d.deb", sha256: "` + strings.Repeat("a", 64) + `"}]`,
		"version missing": `
name: d
description: x
category: utils
trust: upstream-tls
source:
  appimage:
    url: "https://e.com/{version}/a.AppImage"
    bin: a`,
		"tag pattern": `
name: d
description: x
category: utils
trust: upstream-tls
version: {from: git-tags, repo: "https://github.com/a/b", tag: "v1"}
source:
  appimage:
    url: "https://e.com/{version}/a.AppImage"
    bin: a`,
		"bad dest": `
name: d
description: x
category: utils
files: [{dest: /usr/lib/x, content: y}]`,
		"traversal": `
name: d
description: x
category: utils
files: [{dest: "~/../etc/x", content: y}]`,
		"bad category": `
name: d
description: x
category: nope
files: [{dest: /etc/x, content: y}]`,
		"two sources": `
name: d
description: x
category: utils
source:
  apt: {packages: [a]}
  git: {repo: "https://github.com/a/b"}`,
		"unknown dep": `
name: d
description: x
category: utils
depends: [ghost]
files: [{dest: /etc/x, content: y}]`,
		"bin traversal": `
name: d
description: x
category: utils
source:
  appimage:
    url: "https://e.com/a.AppImage"
    sha256: "` + strings.Repeat("a", 64) + `"
    bin: ../../etc/cron.d/x`,
	}
	want := map[string]string{
		"no sha256":       "needs sha256",
		"http url":        "must use https",
		"version missing": "no version source",
		"tag pattern":     "must contain {version}",
		"bad dest":        "must be under",
		"traversal":       "not clean",
		"bad category":    "category",
		"two sources":     "exactly one",
		"unknown dep":     "unknown package",
		"bin traversal":   "plain command name",
	}
	for name, doc := range cases {
		_, _, err := load(t, map[string]string{"packages/d.yaml": doc})
		if err == nil || !strings.Contains(err.Error(), want[name]) {
			t.Errorf("%s: want error containing %q, got %v", name, want[name], err)
		}
	}
}

func TestAutoUpdatingAllowed(t *testing.T) {
	_, _, err := load(t, map[string]string{"packages/a.yaml": `
name: protonup-qt
description: x
category: gaming
trust: upstream-tls
version: {from: git-tags, repo: "https://github.com/DavidoTek/ProtonUp-Qt", tag: "v{version}"}
source:
  appimage:
    url: "https://github.com/DavidoTek/ProtonUp-Qt/releases/download/v{version}/ProtonUp-Qt-{version}-x86_64.AppImage"
    bin: protonup-qt
`})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatrix(t *testing.T) {
	c, _, err := load(t, map[string]string{"packages/fonts.yaml": `
matrix:
  - {font: jetbrains-mono, archive: JetBrainsMono}
  - {font: fira-code, archive: FiraCode}
name: fonts-nerd-{{font}}
description: "{{archive}} Nerd Font"
category: fonts
trust: upstream-tls
version: {from: git-tags, repo: "https://github.com/ryanoasis/nerd-fonts", tag: "v{version}"}
source:
  archive:
    url: "https://github.com/ryanoasis/nerd-fonts/releases/download/v{version}/{{archive}}.tar.xz"
hooks:
  install: install -Dm644 -t "$DESTDIR/usr/local/share/fonts/{{archive}}" *.ttf
`})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := c.Get("fonts-nerd-fira-code")
	if !ok || !strings.Contains(p.Source.Archive.URL, "FiraCode.tar.xz") {
		t.Fatalf("matrix expansion: %+v", p)
	}
	if _, ok := c.Get("fonts-nerd-jetbrains-mono"); !ok {
		t.Fatal("missing first matrix entry")
	}
}

func TestOverlayOverrides(t *testing.T) {
	base := fstest.MapFS{"packages/firefox.yaml": {Data: []byte(firefox)}}
	over := fstest.MapFS{"firefox.yaml": {Data: []byte(strings.Replace(firefox, "description: Firefox", "description: Mine", 1))}}
	c, warns, err := Load(
		Layer{Name: "embedded", FS: base, PkgDir: "packages", FilesDir: "files"},
		Layer{Name: "/etc/debforge/packages.d", FS: over, PkgDir: ".", FilesDir: "files"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := c.Get("firefox"); p.Description != "Mine" || len(warns) != 1 {
		t.Fatalf("override: %q %v", p.Description, warns)
	}
}

func TestSelect(t *testing.T) {
	c, _, err := load(t, map[string]string{
		"packages/a.yaml": firefox,
		"packages/b.yaml": strings.NewReplacer("firefox", "chromium", "mozilla", "x").Replace(firefox),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Select([]string{"@browsers", "firefox"})
	if err != nil || strings.Join(got, ",") != "chromium,firefox" {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := c.Select([]string{"nope*"}); err == nil {
		t.Fatal("empty glob should error")
	}
	if _, err := c.Select([]string{"ghost"}); err == nil {
		t.Fatal("unknown should error")
	}
}

func TestReloadsMatchSystem(t *testing.T) {
	got := append([]string(nil), Reloads...)
	want := append([]string(nil), system.ReloadOrder...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("catalog.Reloads %v != system.ReloadOrder %v", got, want)
	}
}
