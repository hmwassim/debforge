package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/state"
)

func setup(t *testing.T) (*Engine, string) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "home/alice"), 0o755)
	return &Engine{Root: root}, root
}

func sys(path, data string) Spec {
	return Spec{Path: path, Data: []byte(data), Mode: 0o644, UID: os.Getuid(), GID: os.Getgid()}
}

func user(path, data string) Spec {
	s := sys(path, data)
	s.Home = "/home/alice"
	return s
}

func put(t *testing.T, root, p, data string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(root+p), 0o755)
	if err := os.WriteFile(root+p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, root, p string) string {
	t.Helper()
	b, err := os.ReadFile(root + p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func rec(data string) *state.File { return &state.File{SHA256: Hash([]byte(data))} }

func TestClassifyTable(t *testing.T) {
	cases := []struct {
		disk     *string
		base     *string
		new      string
		want     State
		describe string
	}{
		{nil, nil, "n", Missing, "absent"},
		{ptr("n"), nil, "n", Adoptable, "same content, no baseline"},
		{ptr("x"), nil, "n", Foreign, "different content, no baseline"},
		{ptr("b"), ptr("b"), "b", Current, "nothing changed"},
		{ptr("b"), ptr("b"), "n", Outdated, "we changed"},
		{ptr("u"), ptr("b"), "b", UserModified, "user changed"},
		{ptr("u"), ptr("b"), "n", Conflict, "both changed"},
		{ptr("n"), ptr("b"), "n", Current, "user applied the same change"},
	}
	for _, c := range cases {
		var r *state.File
		if c.base != nil {
			r = rec(*c.base)
		}
		var disk []byte
		if c.disk != nil {
			disk = []byte(*c.disk)
		}
		if got := classify(disk, c.disk != nil, Hash([]byte(c.new)), r); got != c.want {
			t.Errorf("%s: got %v want %v", c.describe, got, c.want)
		}
	}
}

func ptr(s string) *string { return &s }

func TestApplyStates(t *testing.T) {
	e, root := setup(t)

	// Missing -> written, mode applied
	s := sys("/etc/a.conf", "v1")
	s.Mode = 0o600
	o, err := e.Apply(s, nil, false)
	if err != nil || o.Action != "written" || get(t, root, "/etc/a.conf") != "v1" {
		t.Fatalf("missing: %+v %v", o, err)
	}
	if fi, _ := os.Stat(root + "/etc/a.conf"); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	r := o.Record

	// Outdated -> written
	o, _ = e.Apply(sys("/etc/a.conf", "v2"), &r, false)
	if o.Action != "written" || get(t, root, "/etc/a.conf") != "v2" {
		t.Fatalf("outdated: %+v", o)
	}
	r = o.Record

	// User edits; we ship same content -> kept, baseline unchanged
	put(t, root, "/etc/a.conf", "mine")
	o, _ = e.Apply(sys("/etc/a.conf", "v2"), &r, false)
	if o.Action != "kept" || get(t, root, "/etc/a.conf") != "mine" || o.Record.SHA256 != r.SHA256 {
		t.Fatalf("user-modified: %+v", o)
	}

	// Conflict -> sidecar, baseline moves to new so the user's merge sticks
	o, _ = e.Apply(sys("/etc/a.conf", "v3"), &r, false)
	if o.Action != "sidecar" || get(t, root, "/etc/a.conf.debforge-new") != "v3" || get(t, root, "/etc/a.conf") != "mine" {
		t.Fatalf("conflict: %+v", o)
	}
	r = o.Record
	o, _ = e.Apply(sys("/etc/a.conf", "v3"), &r, false)
	if o.Action != "kept" {
		t.Fatalf("after conflict, re-run should keep user's file: %+v", o)
	}

	// Force -> backup + replace
	o, _ = e.Apply(sys("/etc/a.conf", "v4"), &r, true)
	if o.Action != "replaced" || get(t, root, "/etc/a.conf") != "v4" || get(t, root, "/etc/a.conf.debforge-old") != "mine" {
		t.Fatalf("force: %+v", o)
	}
}

func TestForeignBackupAndRestore(t *testing.T) {
	e, root := setup(t)
	put(t, root, "/etc/apt/sources.list", "distro")
	o, err := e.Apply(sys("/etc/apt/sources.list", "ours"), nil, false)
	if err != nil || o.Action != "replaced" || o.Record.Backup != "/etc/apt/sources.list.debforge-orig" {
		t.Fatalf("%+v %v", o, err)
	}
	if get(t, root, "/etc/apt/sources.list.debforge-orig") != "distro" {
		t.Fatal("backup content")
	}
	o, err = e.Remove(sys("/etc/apt/sources.list", "ours"), o.Record)
	if err != nil || o.Action != "restored" || get(t, root, "/etc/apt/sources.list") != "distro" {
		t.Fatalf("restore: %+v %v", o, err)
	}
}

func TestRemoveKeepsModified(t *testing.T) {
	// Regression: the old code deleted user-modified files and kept
	// untouched ones when no hash was stored.
	e, root := setup(t)
	o, _ := e.Apply(sys("/etc/b.conf", "ours"), nil, false)
	put(t, root, "/etc/b.conf", "edited")
	o2, err := e.Remove(sys("/etc/b.conf", ""), o.Record)
	if err != nil || o2.Action != "kept" || get(t, root, "/etc/b.conf") != "edited" {
		t.Fatalf("%+v %v", o2, err)
	}
	put(t, root, "/etc/c.conf", "x")
	o, _ = e.Apply(sys("/etc/c.conf", "x"), nil, false)
	if o.Action != "adopted" {
		t.Fatalf("adopt: %+v", o)
	}
	o2, _ = e.Remove(sys("/etc/c.conf", ""), o.Record)
	if o2.Action != "deleted" || get(t, root, "/etc/c.conf") != "<missing>" {
		t.Fatalf("delete: %+v", o2)
	}
}

func TestUserFileCreatesDirs(t *testing.T) {
	e, root := setup(t)
	o, err := e.Apply(user("/home/alice/.config/foo/bar/x.conf", "a"), nil, false)
	if err != nil || o.Action != "written" {
		t.Fatalf("%+v %v", o, err)
	}
	if get(t, root, "/home/alice/.config/foo/bar/x.conf") != "a" {
		t.Fatal("content")
	}
}

func TestUserFileRefusesSymlink(t *testing.T) {
	e, root := setup(t)
	os.MkdirAll(root+"/etc/secret", 0o755)
	os.Symlink(root+"/etc/secret", root+"/home/alice/.config")
	_, err := e.Apply(user("/home/alice/.config/x.conf", "pwn"), nil, false)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
	if get(t, root, "/etc/secret/x.conf") != "<missing>" {
		t.Fatal("wrote through symlink")
	}
}

func TestUserFileSymlinkLeafReplaced(t *testing.T) {
	e, root := setup(t)
	put(t, root, "/etc/shadow", "root-only")
	os.Symlink(root+"/etc/shadow", root+"/home/alice/.bashrc")
	o, err := e.Apply(user("/home/alice/.bashrc", "ours"), nil, false)
	if err != nil || o.Action != "replaced" {
		t.Fatalf("%+v %v", o, err)
	}
	if get(t, root, "/etc/shadow") != "root-only" {
		t.Fatal("followed leaf symlink")
	}
	if fi, _ := os.Lstat(root + "/home/alice/.bashrc"); fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("symlink not replaced")
	}
}

func TestUserPathOutsideHome(t *testing.T) {
	e, _ := setup(t)
	if _, err := e.Apply(user("/etc/x", "a"), nil, false); err == nil {
		t.Fatal("expected outside-home error")
	}
}
