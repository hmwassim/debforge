package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hmwassim/debforge/internal/apt"
	"github.com/hmwassim/debforge/internal/files"
	"github.com/hmwassim/debforge/internal/state"
	"github.com/hmwassim/debforge/internal/system"
	"github.com/hmwassim/debforge/internal/system/systemtest"
)

func profile(t *testing.T, yaml string, extra map[string]string) *Profile {
	t.Helper()
	m := fstest.MapFS{"profiles/p.yaml": {Data: []byte(yaml)}}
	for k, v := range extra {
		m["files/setup/"+k] = &fstest.MapFile{Data: []byte(v)}
	}
	p, err := Load(m, "p")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type env struct {
	e    *Engine
	r    *systemtest.Runner
	root string
}

func newEnv(t *testing.T, dpkg string) *env {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "home/alice"), 0o755)
	r := &systemtest.Runner{}
	real := system.ExecRunner{}
	r.On("sh -eu -c", func(c system.Cmd) (system.Result, error) { return real.Run(context.Background(), c) })
	snap := system.ParseSnapshot("amd64", dpkg)
	return &env{root: root, r: r, e: &Engine{
		R: r, Apt: &apt.Apt{R: r}, Files: &files.Engine{Root: root}, State: state.New(), Snap: snap,
		User: &system.User{Name: "alice", UID: os.Getuid(), GID: os.Getgid(), Home: "/home/alice"},
		Root: root, Sleep: func(time.Duration) {},
	}}
}

func (v *env) read(p string) string {
	b, err := os.ReadFile(v.root + p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestSourcesBuiltin(t *testing.T) {
	v := newEnv(t, "")
	os.MkdirAll(v.root+"/etc/apt", 0o755)
	os.WriteFile(v.root+"/etc/apt/sources.list", []byte("deb http://deb.debian.org/debian trixie main\n"), 0o644)
	v.r.OK("apt-get", "")
	p := profile(t, "steps:\n  - {id: sources, title: Sources, builtin: sources}\n", nil)
	s := p.Steps[0]
	if r := v.e.Check(context.Background(), s); r.Status != Needed {
		t.Fatalf("check: %+v", r)
	}
	if _, err := v.e.Apply(context.Background(), s, false, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.read("/etc/apt/sources.list.d/debian.sources"), "trixie-backports") {
		t.Fatal("deb822 sources not written")
	}
	if !strings.Contains(v.read("/etc/apt/sources.list.debforge-orig"), "deb http") || strings.Contains(v.read("/etc/apt/sources.list"), "\ndeb ") {
		t.Fatal("legacy list not neutralised with backup")
	}
	if !v.r.Ran("apt-get -q -o DPkg::Lock::Timeout=300 update") {
		t.Fatal("apt update not run after changing sources")
	}
	if r := v.e.Check(context.Background(), s); r.Status != OK {
		t.Fatalf("second check: %+v", r)
	}
}

func TestBackportsCheckUsesSimulation(t *testing.T) {
	// A stock kernel is installed, but backports has a newer one: not OK.
	v := newEnv(t, "linux-image-amd64\tamd64\tii \t6.12.1\n")
	v.r.OK("apt-get -s -q -t trixie-backports install linux-image-amd64", "Inst linux-image-amd64 [6.12.1] (7.2.6-1~bpo13+1)\n")
	p := profile(t, "steps:\n  - {id: kernel, title: Kernel, backports: true, packages: [linux-image-amd64]}\n", nil)
	if r := v.e.Check(context.Background(), p.Steps[0]); r.Status != Needed || !strings.Contains(r.Reasons[0], "backports") {
		t.Fatalf("%+v", r)
	}
}

func TestWhen(t *testing.T) {
	p := profile(t, `steps:
  - {id: a, title: A, when: {cpu_vendor: amd}, packages: [amd64-microcode]}
  - {id: b, title: B, when: {network_manager: true}, packages: [x]}
`, nil)
	f := Facts{CPUVendor: "intel"}
	if p.Steps[0].Applies(f) || p.Steps[1].Applies(f) {
		t.Fatal("when not honoured")
	}
	f = Facts{CPUVendor: "amd", NetworkManager: true}
	if !p.Steps[0].Applies(f) || !p.Steps[1].Applies(f) {
		t.Fatal("when should match")
	}
}

func TestWhenChassis(t *testing.T) {
	p := profile(t, `steps:
  - id: a
    title: A
    when: {chassis: laptop}
    packages: [x]
  - id: b
    title: B
    files:
      - {dest: /etc/both, content: "1\n"}
      - {dest: /etc/desk, content: "2\n", when: {chassis: desktop}}
`, nil)
	if p.Steps[0].Applies(Facts{Chassis: "desktop"}) || !p.Steps[0].Applies(Facts{Chassis: "laptop"}) {
		t.Fatal("step chassis not honoured")
	}
	v := newEnv(t, "")
	v.e.Facts = Facts{Chassis: "laptop"}
	if _, err := v.e.Apply(context.Background(), p.Steps[1], false, nil); err != nil {
		t.Fatal(err)
	}
	if v.read("/etc/both") != "1\n" || v.read("/etc/desk") != "<missing>" {
		t.Fatal("file chassis not honoured")
	}
	if r := v.e.Check(context.Background(), p.Steps[1]); r.Status != OK {
		t.Fatalf("skipped file must not be reported: %+v", r)
	}
}

func TestDetectFacts(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(root+"/proc", 0o755)
	os.WriteFile(root+"/proc/cpuinfo", []byte("vendor_id\t: AuthenticAMD\n"), 0o644)
	f := DetectFacts(root, system.ParseSnapshot("amd64", "network-manager\tamd64\tii \t1\nplasma-workspace\tamd64\tii \t6\n"))
	if f.CPUVendor != "amd" || !f.NetworkManager || f.Desktop != "kde" {
		t.Fatalf("%+v", f)
	}
}

func TestFilesServicesCommandsVerify(t *testing.T) {
	v := newEnv(t, "systemd-resolved\tamd64\tii \t1\n")
	enabled := "disabled"
	v.r.On("systemctl is-enabled", func(system.Cmd) (system.Result, error) {
		return system.Result{Stdout: []byte(enabled + "\n")}, nil
	})
	v.r.OK("systemctl is-active", "active\n")
	v.r.On("systemctl enable", func(system.Cmd) (system.Result, error) { enabled = "enabled"; return system.Result{}, nil })
	v.r.OK("systemctl daemon-reload", "")
	v.r.OK("systemctl restart", "")
	marker := filepath.Join(v.root, "applied")
	p := profile(t, `steps:
  - id: resolved
    title: DNS
    packages: [systemd-resolved]
    files: [{dest: /etc/systemd/resolved.conf.d/x.conf, src: x.conf}]
    services: [{name: systemd-resolved.service, enable: true, start: true, restart_on_change: true}]
    commands: [{check: "test -e `+marker+`", apply: "touch `+marker+`"}]
    verify: "true"
`, map[string]string{"x.conf": "[Resolve]\n"})
	s := p.Steps[0]
	r := v.e.Check(context.Background(), s)
	if r.Status != Needed || len(r.Reasons) != 3 {
		t.Fatalf("check: %+v", r)
	}
	if _, err := v.e.Apply(context.Background(), s, false, nil); err != nil {
		t.Fatal(err)
	}
	if !v.r.Ran("systemctl restart systemd-resolved.service") || !v.r.Ran("systemctl enable systemd-resolved.service") {
		t.Fatalf("calls: %v", v.r.Commands())
	}
	if r := v.e.Check(context.Background(), s); r.Status != OK {
		t.Fatalf("after apply: %+v", r)
	}
}

func TestVerifyFailureIsError(t *testing.T) {
	v := newEnv(t, "")
	p := profile(t, "steps:\n  - {id: dns, title: DNS, verify: \"false\"}\n", nil)
	if _, err := v.e.Apply(context.Background(), p.Steps[0], false, nil); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestUserModifiedNotOverwritten(t *testing.T) {
	v := newEnv(t, "")
	p := profile(t, "steps:\n  - {id: f, title: F, files: [{dest: /etc/x.conf, src: x.conf}]}\n", map[string]string{"x.conf": "a\n"})
	s := p.Steps[0]
	v.e.Apply(context.Background(), s, false, nil)
	os.WriteFile(v.root+"/etc/x.conf", []byte("mine\n"), 0o644)
	if r := v.e.Check(context.Background(), s); r.Status != Modified {
		t.Fatalf("%+v", r)
	}
	v.e.Apply(context.Background(), s, false, nil)
	if v.read("/etc/x.conf") != "mine\n" {
		t.Fatal("overwritten without --force")
	}
	v.e.Apply(context.Background(), s, true, nil)
	if v.read("/etc/x.conf") != "a\n" || v.read("/etc/x.conf.debforge-old") != "mine\n" {
		t.Fatal("--force should replace with backup")
	}
}

func TestProfileValidation(t *testing.T) {
	bad := map[string]string{
		"dup id":       "steps:\n  - {id: a, title: A}\n  - {id: a, title: B}\n",
		"unknown key":  "steps:\n  - {id: a, title: A, pakages: [x]}\n",
		"bad builtin":  "steps:\n  - {id: a, title: A, builtin: nope}\n",
		"bad dest":     "steps:\n  - {id: a, title: A, files: [{dest: /usr/lib/x, content: y}]}\n",
		"bad unit":     "steps:\n  - {id: a, title: A, services: [{name: \"x; rm\"}]}\n",
		"missing file": "steps:\n  - {id: a, title: A, files: [{dest: /etc/x, src: nope}]}\n",
		"bad chassis":  "steps:\n  - {id: a, title: A, when: {chassis: tower}}\n",
		"file chassis": "steps:\n  - {id: a, title: A, files: [{dest: /etc/x, content: y, when: {chassis: tower}}]}\n",
	}
	for name, y := range bad {
		if _, err := Load(fstest.MapFS{"profiles/p.yaml": {Data: []byte(y)}}, "p"); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestStepChangedNote(t *testing.T) {
	v := newEnv(t, "")
	p := profile(t, "steps:\n  - {id: z, title: Z, files: [{dest: /etc/z.conf, content: a}], notes: {changed: reboot to apply}}\n", nil)
	notes, err := v.e.Apply(context.Background(), p.Steps[0], false, nil)
	if err != nil || len(notes) != 1 || notes[0] != "reboot to apply" {
		t.Fatalf("%v %v", notes, err)
	}
	notes, _ = v.e.Apply(context.Background(), p.Steps[0], false, nil)
	if len(notes) != 0 {
		t.Fatalf("unchanged step must not repeat the note: %v", notes)
	}
}

func TestRetiredFilesRemoved(t *testing.T) {
	old := profile(t, `steps:
  - id: a
    title: A
    files:
      - {dest: /etc/keep.conf, content: "k\n"}
      - {dest: /etc/udev/rules.d/90-old.rules, content: "o\n"}
      - {dest: /etc/edited.conf, content: "e\n"}
`, nil)
	v := newEnv(t, "")
	v.r.On("udevadm", func(system.Cmd) (system.Result, error) { return system.Result{}, nil })
	if _, err := v.e.Apply(context.Background(), old.Steps[0], false, nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(v.root+"/etc/edited.conf", []byte("mine\n"), 0o644)

	cur := profile(t, `steps:
  - id: a
    title: A
    files:
      - {dest: /etc/keep.conf, content: "k\n"}
`, nil)
	ret := cur.Steps[len(cur.Steps)-1]
	if !ret.Internal() {
		t.Fatal("the retired step must be appended")
	}
	r := v.e.Check(context.Background(), ret)
	if r.Status != Needed || len(r.Reasons) != 2 {
		t.Fatalf("check: %+v", r)
	}
	notes, err := v.e.Apply(context.Background(), ret, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.read("/etc/udev/rules.d/90-old.rules") != "<missing>" || v.read("/etc/keep.conf") != "k\n" {
		t.Fatal("wrong files removed")
	}
	if v.read("/etc/edited.conf") != "mine\n" || len(notes) != 1 {
		t.Fatalf("edited file must be kept with a note: %v", notes)
	}
	if !v.r.Ran("udevadm control --reload") {
		t.Fatalf("udev not reloaded: %v", v.r.Commands())
	}
	if len(v.e.State.Setup.Files) != 1 {
		t.Fatalf("records: %v", v.e.State.Setup.Files)
	}
	if r := v.e.Check(context.Background(), ret); r.Status != NotApplicable {
		t.Fatalf("nothing left to retire: %+v", r)
	}
}

func TestRetiredIDReserved(t *testing.T) {
	if _, err := Load(fstest.MapFS{"profiles/p.yaml": {Data: []byte("steps:\n  - {id: retired, title: R}\n")}}, "p"); err == nil {
		t.Fatal("retired must be reserved")
	}
}
