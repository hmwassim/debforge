package execute

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/plan"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/system/systemtest"
	"github.com/hmwassim/debforge/internal/ui"
)

type harness struct {
	t    *testing.T
	root string
	r    *systemtest.Runner
	env  *plan.Env
	x    *Exec
}

func newHarness(t *testing.T, defs map[string]string) *harness {
	t.Helper()
	m := fstest.MapFS{}
	for k, v := range defs {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	cat, _, err := catalog.Load(catalog.Layer{Name: "t", FS: m, PkgDir: "packages", FilesDir: "files"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "home/alice"), 0o755)
	r := &systemtest.Runner{}
	real := system.ExecRunner{}
	// Hooks and tar run for real (inside temp dirs); apt and friends are faked.
	r.On("sh -eu -c", func(c system.Cmd) (system.Result, error) { return real.Run(context.Background(), c) })
	r.On("apt-get", func(c system.Cmd) (system.Result, error) { return system.Result{}, nil })
	r.On("debconf-set-selections", func(c system.Cmd) (system.Result, error) { return system.Result{}, nil })

	st := state.New()
	user := &system.User{Name: "alice", UID: os.Getuid(), GID: os.Getgid(), Home: "/home/alice"}
	fe := &files.Engine{Root: root}
	snap := system.ParseSnapshot("amd64", "")
	u := ui.New(ui.Options{Out: io.Discard, Stdout: io.Discard})
	h := &harness{t: t, root: root, r: r}
	h.env = &plan.Env{Cat: cat, State: st, Snap: snap, Files: fe, User: user, Versions: pinned{}}
	h.x = &Exec{
		R: r, Apt: &apt.Apt{R: r}, Extrepo: &apt.Extrepo{R: r, SourcesDir: filepath.Join(root, "sources")},
		Files: fe, UI: u, Log: ui.OpenLog(""), Store: state.Store{Path: filepath.Join(root, "state.json")},
		State: st, Snap: snap, User: user, WorkDir: filepath.Join(root, "work"),
	}
	return h
}

type pinned struct{}

func (pinned) Resolve(_ context.Context, p *catalog.Package) (string, error) {
	return p.Version.Pin, nil
}

func (h *harness) install(names ...string) (*Summary, error) {
	h.t.Helper()
	pl, err := h.env.Install(context.Background(), names, plan.Options{})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.x.Install(context.Background(), pl)
}

func (h *harness) read(p string) string {
	b, err := os.ReadFile(h.root + p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

const wineDef = `
name: wine
description: x
category: gaming
source:
  apt: {packages: [wine]}
debconf: ["wine wine/question boolean true"]
files:
  - {dest: /etc/modules-load.d/ntsync.conf, content: "ntsync\n"}
  - {dest: "~/.config/wine.conf", content: "a=1\n"}
`

func TestInstallAptWithFiles(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/wine.yaml": wineDef})
	if _, err := h.install("wine"); err != nil {
		t.Fatal(err)
	}
	if !h.r.Ran("apt-get -y -q") || !h.r.Ran("debconf-set-selections") {
		t.Fatalf("calls: %v", h.r.Commands())
	}
	if h.read("/etc/modules-load.d/ntsync.conf") != "ntsync\n" || h.read("/home/alice/.config/wine.conf") != "a=1\n" {
		t.Fatal("files not written")
	}
	sp := h.x.State.Packages["wine"]
	if sp == nil || sp.Incomplete || len(sp.Files) != 2 || strings.Join(sp.AptPackages, ",") != "wine" {
		t.Fatalf("state: %+v", sp)
	}
	// Persisted.
	st, _, _ := h.x.Store.Load()
	if st.Packages["wine"] == nil {
		t.Fatal("state not saved")
	}
}

const gitDef = `
name: tool
description: x
category: utils
version: {from: pin, pin: "1.2"}
source:
  git: {repo: "https://example.com/tool.git", ref: "v{version}"}
hooks:
  build: echo built > out.txt
  install: |
    install -Dm755 out.txt "$DESTDIR/usr/local/bin/tool"
    ln -s tool "$DESTDIR/usr/local/bin/tool-alias"
`

func (h *harness) fakeClone() {
	h.r.On("git clone", func(c system.Cmd) (system.Result, error) {
		dest := c.Args[len(c.Args)-1]
		return system.Result{}, os.MkdirAll(dest, 0o755)
	})
}

func TestGitBuildStaged(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/tool.yaml": gitDef})
	h.fakeClone()
	if _, err := h.install("tool"); err != nil {
		t.Fatal(err)
	}
	if !h.r.Ran("git clone --depth 1 --branch v1.2 -- https://example.com/tool.git") {
		t.Fatalf("clone args: %v", h.r.Commands())
	}
	if h.read("/usr/local/bin/tool") != "built\n" {
		t.Fatal("binary not placed")
	}
	if l, _ := os.Readlink(h.root + "/usr/local/bin/tool-alias"); l != "tool" {
		t.Fatal("symlink not placed")
	}
	sp := h.x.State.Packages["tool"]
	if sp.Version != "1.2" || len(sp.Files) != 2 {
		t.Fatalf("%+v", sp)
	}

	// Removal deletes exactly what was installed.
	pl, err := h.env.Remove([]string{"tool"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.x.Remove(context.Background(), pl); err != nil {
		t.Fatal(err)
	}
	if h.read("/usr/local/bin/tool") != "<missing>" || h.x.State.Packages["tool"] != nil {
		t.Fatal("not removed")
	}
}

func TestStagedOutsidePrefixRejected(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/tool.yaml": strings.NewReplacer(
		`install -Dm755 out.txt "$DESTDIR/usr/local/bin/tool"`, `install -Dm755 out.txt "$DESTDIR/usr/bin/tool"`,
		`ln -s tool "$DESTDIR/usr/local/bin/tool-alias"`, `true`).Replace(gitDef)})
	h.fakeClone()
	_, err := h.install("tool")
	if err == nil || !strings.Contains(err.Error(), "/usr/bin/tool") {
		t.Fatalf("err = %v", err)
	}
	if h.read("/usr/bin/tool") != "<missing>" {
		t.Fatal("wrote into dpkg territory")
	}
}

func TestFailureLeavesIncomplete(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/tool.yaml": strings.Replace(gitDef, "echo built > out.txt", "exit 3", 1)})
	h.fakeClone()
	_, err := h.install("tool")
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("err = %v", err)
	}
	st, _, _ := h.x.Store.Load()
	if sp := st.Packages["tool"]; sp == nil || !sp.Incomplete {
		t.Fatalf("expected incomplete record, got %+v", sp)
	}
	// The planner retries it.
	pl, _ := h.env.Install(context.Background(), []string{"tool"}, plan.Options{})
	if len(pl.Items) != 1 || pl.Items[0].Op != plan.OpReinstall {
		t.Fatalf("retry plan: %+v", pl.Items)
	}
}

func TestRemoveKeepsModifiedAndCallsApt(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/wine.yaml": wineDef})
	if _, err := h.install("wine"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(h.root+"/home/alice/.config/wine.conf", []byte("mine\n"), 0o644)
	pl, _ := h.env.Remove([]string{"wine"})
	sum, err := h.x.Remove(context.Background(), pl)
	if err != nil {
		t.Fatal(err)
	}
	if !h.r.Ran("apt-get -y -q -o APT::Status-Fd=3") || !strings.Contains(strings.Join(h.r.Commands(), "\n"), "remove wine") {
		t.Fatalf("apt remove not called: %v", h.r.Commands())
	}
	if h.read("/etc/modules-load.d/ntsync.conf") != "<missing>" || h.read("/home/alice/.config/wine.conf") != "mine\n" {
		t.Fatal("file handling on remove")
	}
	if len(sum.Notes) != 1 {
		t.Fatalf("notes: %v", sum.Notes)
	}
}

func TestDefinitionChangeRemovesDroppedFile(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/wine.yaml": wineDef})
	if _, err := h.install("wine"); err != nil {
		t.Fatal(err)
	}
	h2 := newHarness(t, map[string]string{"packages/wine.yaml": strings.Replace(wineDef,
		"  - {dest: /etc/modules-load.d/ntsync.conf, content: \"ntsync\\n\"}\n", "", 1)})
	// Reuse the first harness's state/root with the new catalog.
	h.env.Cat = h2.env.Cat
	h.env.Snap = system.ParseSnapshot("amd64", "wine\tamd64\tii \t1\n")
	pl, err := h.env.Update(context.Background(), []string{"wine"}, plan.Options{})
	if err != nil || len(pl.Items) != 1 {
		t.Fatalf("%v %v", pl, err)
	}
	if _, err := h.x.Install(context.Background(), pl); err != nil {
		t.Fatal(err)
	}
	if h.read("/etc/modules-load.d/ntsync.conf") != "<missing>" {
		t.Fatal("dropped file not removed")
	}
}

func TestUnzipStripAndTraversal(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	writeZip(t, zp, map[string]string{"top/bin/x": "1", "top/README": "r"})
	dest := filepath.Join(dir, "out")
	if err := unzip(zp, dest, 1); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "bin/x")); string(b) != "1" {
		t.Fatal("strip/structure lost")
	}
	evil := filepath.Join(dir, "evil.zip")
	writeZip(t, evil, map[string]string{"../../etc/x": "pwn"})
	if err := unzip(evil, filepath.Join(dir, "out2"), 0); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestInterruptAfterAptRecordsIncomplete(t *testing.T) {
	h := newHarness(t, map[string]string{"packages/wine.yaml": wineDef})
	ctx, cancel := context.WithCancel(context.Background())
	h.r.On("apt-get", func(system.Cmd) (system.Result, error) { cancel(); return system.Result{}, nil })
	pl, err := h.env.Install(context.Background(), []string{"wine"}, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.x.Install(ctx, pl); err == nil {
		t.Fatal("expected interruption error")
	}
	st, _, _ := h.x.Store.Load()
	if sp := st.Packages["wine"]; sp == nil || !sp.Incomplete {
		t.Fatalf("apt-installed package must be recorded as incomplete, got %+v", sp)
	}
}
