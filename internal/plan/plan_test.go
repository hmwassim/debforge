package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
)

const defs = `
--- firefox
name: firefox
description: x
category: browsers
source:
  apt: {packages: [firefox], extrepo: [mozilla], conflicts: [firefox-esr]}
--- nvidia
name: nvidia
description: x
category: system
hardware: {pci_vendor: "10de"}
source:
  apt:
    packages: [nvtop]
    variants: {open: [nvidia-open], proprietary: [cuda-drivers]}
--- nvflux
name: nvflux
description: x
category: utils
depends: [nvidia]
version: {from: pin, pin: "1.0"}
source:
  git: {repo: "https://github.com/x/nvflux", ref: "v{version}"}
hooks: {install: "make install DESTDIR=$DESTDIR"}
--- wine
name: wine
description: x
category: gaming
source:
  apt: {packages: [wine, libwine]}
files:
  - {dest: /etc/modules-load.d/ntsync.conf, content: "ntsync\n"}
--- lutris
name: lutris
description: x
category: gaming
depends: [wine]
requires: [python3-gi]
trust: upstream-tls
version: {from: git-tags, repo: "https://github.com/lutris/lutris", tag: "v{version}"}
source:
  deb:
    urls: [{url: "https://github.com/lutris/lutris/releases/download/v{version}/lutris_{version}_all.deb"}]
--- bash
name: config-bash
description: x
category: desktop
requires: [eza]
files:
  - {dest: "~/.config/bashrc.d/10-aliases.sh", content: "alias ls=eza\n"}
`

type fixedVersions map[string]string

func (f fixedVersions) Resolve(_ context.Context, p *catalog.Package) (string, error) {
	return f[p.Name], nil
}

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	m := fstest.MapFS{}
	for _, doc := range strings.Split(defs, "\n--- ")[1:] {
		name, body, _ := strings.Cut(doc, "\n")
		m["packages/"+name+".yaml"] = &fstest.MapFile{Data: []byte(body)}
	}
	c, _, err := catalog.Load(catalog.Layer{Name: "t", FS: m, PkgDir: "packages", FilesDir: "files"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func env(t *testing.T, st *state.State, dpkg string) *Env {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "home/alice"), 0o755)
	return &Env{
		Cat: testCatalog(t), State: st,
		Snap:        system.ParseSnapshot("amd64", dpkg),
		Versions:    fixedVersions{"lutris": "0.5.19", "nvflux": "1.0"},
		Files:       &files.Engine{Root: root},
		User:        &system.User{Name: "alice", UID: os.Getuid(), GID: os.Getgid(), Home: "/home/alice"},
		HasHardware: func(h *catalog.Hardware) bool { return h.PCIVendor == "10de" },
	}
}

func names(p *Plan) string {
	var s []string
	for _, it := range p.Items {
		s = append(s, string(it.Op)+":"+it.Name)
	}
	return strings.Join(s, " ")
}

func TestInstallWithDeps(t *testing.T) {
	e := env(t, state.New(), "python3-gi\tall\tii \t1\n")
	p, err := e.Install(context.Background(), []string{"lutris"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if names(p) != "install:wine install:lutris" {
		t.Fatalf("items = %s", names(p))
	}
	if p.Items[0].Reason != "dependency of lutris" || p.Items[0].Explicit || !p.Items[1].Explicit {
		t.Fatalf("explicit/reason wrong: %+v", p.Items[0])
	}
	if strings.Join(p.AptInstall, " ") != "wine libwine python3-gi" {
		t.Fatalf("apt = %v", p.AptInstall)
	}
	// python3-gi was already installed and is claimed by nobody: never record it.
	if strings.Join(p.Items[1].AptRecord, " ") != "" {
		t.Fatalf("AptRecord = %v", p.Items[1].AptRecord)
	}
	if p.Items[1].Version != "0.5.19" {
		t.Fatal("version")
	}
	if p.Items[0].Files[0].State != files.Missing {
		t.Fatal("file state")
	}
}

func TestConflictsAndExtrepo(t *testing.T) {
	e := env(t, state.New(), "firefox-esr\tamd64\tii \t1\n")
	p, err := e.Install(context.Background(), []string{"firefox"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.AptConflicts, ",") != "firefox-esr" || strings.Join(p.Extrepos, ",") != "mozilla" {
		t.Fatalf("%+v", p)
	}
	out := p.Render(Plain, "/home/alice")
	for _, w := range []string{"Install:\n  firefox", "apt remove (conflicts): firefox-esr", "enable repositories: mozilla"} {
		if !strings.Contains(out, w) {
			t.Errorf("render missing %q:\n%s", w, out)
		}
	}
}

func TestVariantRequired(t *testing.T) {
	e := env(t, state.New(), "")
	if _, err := e.Install(context.Background(), []string{"nvflux"}, Options{}); err == nil || !strings.Contains(err.Error(), "--variant nvidia=") {
		t.Fatalf("err = %v", err)
	}
	need, _ := e.NeedsVariant([]string{"nvflux"}, Options{})
	if len(need) != 1 || need[0].Name != "nvidia" {
		t.Fatalf("need = %v", need)
	}
	p, err := e.Install(context.Background(), []string{"nvflux"}, Options{Variants: map[string]string{"nvidia": "open"}})
	if err != nil || strings.Join(p.AptInstall, " ") != "nvtop nvidia-open" {
		t.Fatalf("%v %v", p, err)
	}
}

func TestHardwareMissing(t *testing.T) {
	e := env(t, state.New(), "")
	e.HasHardware = func(*catalog.Hardware) bool { return false }
	if _, err := e.Install(context.Background(), []string{"nvidia"}, Options{Variants: map[string]string{"nvidia": "open"}}); err == nil {
		t.Fatal("expected hardware error")
	}
}

func TestHardwareInstalledStillUpdates(t *testing.T) {
	e := env(t, state.New(), "nvtop\tamd64\tii \t1\nnvidia-open\tamd64\tii \t1\n")
	installed(e, "nvidia", "", true, "nvtop", "nvidia-open")
	e.State.Packages["nvidia"].Variant = "open"
	e.HasHardware = func(*catalog.Hardware) bool { return false }
	if _, err := e.Update(context.Background(), []string{"nvidia"}, Options{}); err != nil {
		t.Fatalf("installed package blocked by hardware: %v", err)
	}
}

func TestFitting(t *testing.T) {
	e := env(t, state.New(), "")
	e.HasHardware = func(*catalog.Hardware) bool { return false }
	fit, unfit, err := e.Fitting([]string{"firefox", "nvidia", "nvflux"})
	if err != nil || strings.Join(fit, " ") != "firefox" || strings.Join(unfit, " ") != "nvidia nvflux" {
		t.Fatalf("fit=%v unfit=%v err=%v", fit, unfit, err)
	}
	installed(e, "nvidia", "", true, "nvtop")
	if fit, _, _ := e.Fitting([]string{"nvflux"}); len(fit) != 1 {
		t.Fatal("nvflux should fit once nvidia is installed")
	}
}

func installed(e *Env, name, version string, explicit bool, apt ...string) {
	p, _ := e.Cat.Get(name)
	e.State.Packages[name] = &state.Package{Kind: string(p.Kind()), Version: version, Explicit: explicit, DefHash: p.Hash, AptPackages: apt}
}

func TestAlreadyInstalledSkipped(t *testing.T) {
	e := env(t, state.New(), "wine\tamd64\tii \t1\nlibwine\tamd64\tii \t1\n")
	installed(e, "wine", "", true, "wine", "libwine")
	e.State.Packages["wine"].Files = map[string]state.File{}
	p, err := e.Install(context.Background(), []string{"wine"}, Options{})
	if err != nil || !p.Empty() || len(p.Skipped) != 1 {
		t.Fatalf("%s %v", names(p), err)
	}
	// Package removed behind debforge's back -> reinstall, not an error.
	e.Snap = system.ParseSnapshot("amd64", "wine\tamd64\trc \t1\n")
	p, _ = e.Install(context.Background(), []string{"wine"}, Options{})
	if names(p) != "reinstall:wine" || p.Items[0].Reason != "missing on the system" {
		t.Fatalf("%s", names(p))
	}
}

func TestUpdateVersionAndDefinitionChange(t *testing.T) {
	e := env(t, state.New(), "wine\tamd64\tii \t1\nlibwine\tamd64\tii \t1\npython3-gi\tall\tii \t1\nlutris\tall\tii \t0.5.18\n")
	installed(e, "wine", "", false)
	installed(e, "lutris", "0.5.18", true)
	e.State.Packages["lutris"].Debs = []string{"lutris"}
	p, err := e.Update(context.Background(), nil, Options{})
	if err != nil || names(p) != "upgrade:lutris" || p.Items[0].OldVersion != "0.5.18" {
		t.Fatalf("%s %v", names(p), err)
	}
	e.State.Packages["wine"].DefHash = "old"
	p, _ = e.Update(context.Background(), []string{"wine"}, Options{})
	if names(p) != "upgrade:wine" || p.Items[0].Reason != "definition changed" {
		t.Fatalf("%s", names(p))
	}
}

func TestRemoveDependentsAndOrphans(t *testing.T) {
	e := env(t, state.New(), "")
	installed(e, "wine", "", false, "wine", "libwine")
	installed(e, "lutris", "0.5.19", true, "python3-gi")
	e.State.Packages["lutris"].Debs = []string{"lutris"}
	installed(e, "firefox", "", true, "firefox")

	// Removing wine removes lutris (depends on it), dependents first.
	p, err := e.Remove([]string{"wine"})
	if err != nil {
		t.Fatal(err)
	}
	if names(p) != "remove:lutris remove:wine" || p.Items[0].Reason != "depends on wine" {
		t.Fatalf("%s", names(p))
	}
	if strings.Join(p.AptRemove, " ") != "python3-gi lutris wine libwine" {
		t.Fatalf("apt remove = %v", p.AptRemove)
	}

	// Removing lutris also removes wine, which was only a dependency.
	p, _ = e.Remove([]string{"lutris"})
	if names(p) != "remove:lutris remove:wine" || p.Items[1].Reason != "no longer needed" {
		t.Fatalf("%s", names(p))
	}

	// Unrelated packages are never touched.
	for _, it := range p.Items {
		if it.Name == "firefox" {
			t.Fatal("firefox swept")
		}
	}
}

func TestRemoveRefCountsApt(t *testing.T) {
	e := env(t, state.New(), "")
	installed(e, "wine", "", true, "wine", "libwine")
	installed(e, "config-bash", "", true, "libwine") // pretend both claim libwine
	p, _ := e.Remove([]string{"wine"})
	if strings.Join(p.AptRemove, " ") != "wine" {
		t.Fatalf("apt remove = %v", p.AptRemove)
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	e := env(t, state.New(), "")
	if _, err := e.Remove([]string{"wine"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUserFilesNeedUser(t *testing.T) {
	e := env(t, state.New(), "")
	e.User = nil
	if _, err := e.Install(context.Background(), []string{"config-bash"}, Options{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveCycle(t *testing.T) {
	m := fstest.MapFS{
		"packages/a.yaml": {Data: []byte("name: a\ndescription: x\ncategory: utils\ndepends: [b]\nrequires: [x]\n")},
		"packages/b.yaml": {Data: []byte("name: b\ndescription: x\ncategory: utils\ndepends: [a]\nrequires: [x]\n")},
	}
	c, _, err := catalog.Load(catalog.Layer{Name: "t", FS: m, PkgDir: "packages"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(c, []string{"a"}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateForeignArchOnlyIsNotMissing(t *testing.T) {
	// libwine only exists as i386 here, the way wine32 and steam-libs-i386
	// do in Debian; apt installs it as libwine:i386 when asked for libwine.
	e := env(t, state.New(), "wine\tamd64\tii \t1\nlibwine\ti386\tii \t1\n")
	installed(e, "wine", "", true, "wine", "libwine")
	e.State.Packages["wine"].Files = map[string]state.File{}
	p, err := e.Update(context.Background(), nil, Options{})
	if err != nil || !p.Empty() {
		t.Fatalf("%s %v", names(p), err)
	}
	if strings.Join(p.Skipped, " ") != "wine" {
		t.Fatalf("skipped = %v, want [wine]", p.Skipped)
	}
}

func TestResolveProgress(t *testing.T) {
	e := env(t, state.New(), "wine\tamd64\tii \t1\nlibwine\tamd64\tii \t1\npython3-gi\tall\tii \t1\n")
	var calls []string
	e.Progress = func(name string, done, total int) {
		calls = append(calls, fmt.Sprintf("%s %d/%d", name, done, total))
	}
	if _, err := e.Install(context.Background(), []string{"lutris"}, Options{}); err != nil {
		t.Fatal(err)
	}
	// Only lutris has an upstream version; wine is an apt package.
	if strings.Join(calls, ",") != " 0/1,lutris 1/1" {
		t.Fatalf("progress calls = %q", calls)
	}
}
