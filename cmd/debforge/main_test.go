package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/system/systemtest"
	"github.com/hmwassim/debforge/internal/ui"
)

type fakeTTY struct {
	io.Reader
	io.Writer
}

func (fakeTTY) Close() error { return nil }

type testApp struct {
	*App
	out  *bytes.Buffer
	r    *systemtest.Runner
	root string
	dpkg string
}

const testDefs = `
--- firefox
name: firefox
description: Firefox
category: browsers
source:
  apt: {packages: [firefox], conflicts: [firefox-esr]}
--- nvidia
name: nvidia
description: NVIDIA
category: system
source:
  apt:
    packages: [nvtop]
    variants: {open: [nvidia-open], proprietary: [cuda-drivers]}
--- wine
name: wine
description: Wine
category: gaming
source:
  apt: {packages: [wine]}
files:
  - {dest: /etc/modules-load.d/ntsync.conf, content: "ntsync\n"}
--- lutris
name: lutris
description: Lutris
category: gaming
depends: [wine]
source:
  apt: {packages: [lutris]}
`

func newTestApp(t *testing.T, answers string) *testApp {
	t.Helper()
	m := fstest.MapFS{}
	for _, doc := range strings.Split(testDefs, "\n--- ")[1:] {
		name, body, _ := strings.Cut(doc, "\n")
		m["packages/"+name+".yaml"] = &fstest.MapFile{Data: []byte(body)}
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "home/alice"), 0o755)
	out := &bytes.Buffer{}
	ta := &testApp{out: out, r: &systemtest.Runner{}, root: root}
	in := strings.NewReader(answers)
	u := ui.New(ui.Options{Out: out, Stdout: out, OpenTTY: func() (io.ReadWriteCloser, error) {
		return fakeTTY{Reader: in, Writer: out}, nil
	}})
	ta.App = &App{
		Ctx: context.Background(), UI: u, Log: ui.OpenLog(""), R: ta.r,
		Paths: Paths{
			State: filepath.Join(root, "var/lib/debforge/state.json"), Lock: filepath.Join(root, "lock"),
			Work: filepath.Join(root, "work"), Sources: filepath.Join(root, "sources"), Root: root,
		},
		Getenv:     func(k string) string { return map[string]string{"SUDO_USER": "alice"}[k] },
		Euid:       0,
		Files:      &files.Engine{Root: root},
		EmbeddedFS: m,
		Hardware:   func(*catalog.Hardware) bool { return true },
		Version:    "test",
	}
	ta.r.On("dpkg --print-architecture", func(system.Cmd) (system.Result, error) {
		return system.Result{Stdout: []byte("amd64\n")}, nil
	})
	ta.r.On("dpkg-query", func(system.Cmd) (system.Result, error) {
		return system.Result{Stdout: []byte(ta.dpkg)}, nil
	})
	ta.r.On("apt-get", func(c system.Cmd) (system.Result, error) {
		// Emulate apt: installed packages appear in dpkg's database.
		for _, a := range c.Args {
			if !strings.HasPrefix(a, "-") && a != "install" && a != "remove" && !strings.Contains(a, "=") && !strings.Contains(a, "::") {
				if strings.HasSuffix(a, "-") {
					continue
				}
				ta.dpkg += a + "\tamd64\tii \t1\n"
			}
		}
		return system.Result{}, nil
	})
	ta.r.OK("debconf-set-selections", "")
	return ta
}

func (ta *testApp) run(t *testing.T, args ...string) error {
	t.Helper()
	cmds := commands()
	inv, err := parse(cmds, args)
	if err != nil {
		return err
	}
	return inv.cmd.run(ta.App, inv)
}

func (ta *testApp) state(t *testing.T) *state.State {
	st, _, err := ta.store().Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestInstallConfirmAndRemove(t *testing.T) {
	ta := newTestApp(t, "y\ny\n")
	if err := ta.run(t, "install", "lutris"); err != nil {
		t.Fatalf("%v\n%s", err, ta.out)
	}
	out := ta.out.String()
	for _, want := range []string{"Install:", "wine (dependency of lutris)", "apt install: wine lutris", "Proceed?"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q:\n%s", want, out)
		}
	}
	st := ta.state(t)
	if st.Packages["lutris"] == nil || st.Packages["wine"] == nil || st.Packages["wine"].Explicit {
		t.Fatalf("state: %+v", st.Packages)
	}
	if b, _ := os.ReadFile(ta.root + "/etc/modules-load.d/ntsync.conf"); string(b) != "ntsync\n" {
		t.Fatal("file not written")
	}

	// Removing lutris also removes wine (only a dependency).
	ta.out.Reset()
	if err := ta.run(t, "remove", "lutris"); err != nil {
		t.Fatalf("%v\n%s", err, ta.out)
	}
	if !strings.Contains(ta.out.String(), "wine (no longer needed)") {
		t.Errorf("remove plan:\n%s", ta.out)
	}
	if st := ta.state(t); len(st.Packages) != 0 {
		t.Fatalf("state after remove: %v", st.Names())
	}
	if _, err := os.Stat(ta.root + "/etc/modules-load.d/ntsync.conf"); !os.IsNotExist(err) {
		t.Fatal("file not removed")
	}
}

func TestDeclineChangesNothing(t *testing.T) {
	ta := newTestApp(t, "n\n")
	if err := ta.run(t, "install", "firefox"); err != nil {
		t.Fatal(err)
	}
	if ta.r.Ran("apt-get") || len(ta.state(t).Packages) != 0 {
		t.Fatal("declined plan changed the system")
	}
}

func TestDryRun(t *testing.T) {
	ta := newTestApp(t, "")
	if err := ta.run(t, "install", "--dry-run", "firefox"); err != nil {
		t.Fatal(err)
	}
	if ta.r.Ran("apt-get") || !strings.Contains(ta.out.String(), "firefox") {
		t.Fatal("dry run executed or printed nothing")
	}
}

func TestVariantPromptAndYes(t *testing.T) {
	ta := newTestApp(t, "2\ny\n")
	if err := ta.run(t, "install", "nvidia"); err != nil {
		t.Fatal(err)
	}
	if v := ta.state(t).Packages["nvidia"].Variant; v != "proprietary" {
		t.Fatalf("variant = %q", v)
	}
	ta2 := newTestApp(t, "")
	ta2.UI = ui.New(ui.Options{Out: io.Discard, Stdout: io.Discard, Yes: true})
	err := ta2.run(t, "install", "nvidia")
	if err == nil || !strings.Contains(err.Error(), "--variant nvidia=") {
		t.Fatalf("-y without --variant: %v", err)
	}
	if err := ta2.run(t, "install", "--variant", "nvidia=open", "nvidia"); err != nil {
		t.Fatal(err)
	}
}

func TestAlreadyInstalled(t *testing.T) {
	ta := newTestApp(t, "y\n")
	ta.run(t, "install", "firefox")
	ta.out.Reset()
	ta.r.Calls = nil
	if err := ta.run(t, "install", "firefox"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ta.out.String(), "already installed") || ta.r.Ran("apt-get") {
		t.Fatalf("output: %s", ta.out)
	}
}

func TestNonRootRefused(t *testing.T) {
	ta := newTestApp(t, "")
	ta.Euid = 1000
	if err := ta.run(t, "install", "firefox"); err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("err = %v", err)
	}
	// Read-only commands work.
	if err := ta.run(t, "list"); err != nil {
		t.Fatal(err)
	}
}

func TestParse(t *testing.T) {
	cmds := commands()
	inv, err := parse(cmds, []string{"-yf", "install", "a", "--variant=nvidia=open", "--", "-weird"})
	if err != nil {
		t.Fatal(err)
	}
	if !inv.has("yes") || !inv.has("force") || strings.Join(inv.args, ",") != "a,-weird" || inv.values("variant")[0] != "nvidia=open" {
		t.Fatalf("%+v", inv)
	}
	for _, bad := range [][]string{{"install", "--bogus"}, {"list", "--force"}, {"nope"}, {"install", "--variant"}} {
		if _, err := parse(cmds, bad); err == nil {
			t.Errorf("%v: expected error", bad)
		}
	}
}

func TestSyncForgetsDrift(t *testing.T) {
	ta := newTestApp(t, "y\ny\n")
	ta.run(t, "install", "firefox")
	ta.dpkg = "" // user ran apt remove firefox behind our back
	if err := ta.run(t, "sync"); err != nil {
		t.Fatal(err)
	}
	if len(ta.state(t).Packages) != 0 {
		t.Fatal("drift not forgotten")
	}
}

func TestChecksumFor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SHA256SUMS")
	sum := strings.Repeat("ab", 32)
	os.WriteFile(p, []byte(sum+"  VERSION\n"+sum[:62]+"cd *debforge-linux-amd64\n"), 0o644)
	got, err := checksumFor(p, "debforge-linux-amd64")
	if err != nil || got != sum[:62]+"cd" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := checksumFor(p, "missing"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		ta := newTestApp(t, "")
		if err := ta.run(t, "completion", sh); err != nil {
			t.Fatal(err)
		}
		for _, c := range commands() {
			if !strings.Contains(ta.out.String(), c.name) {
				t.Errorf("%s completion lacks %s", sh, c.name)
			}
		}
	}
}
