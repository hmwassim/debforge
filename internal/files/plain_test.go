package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceAndRemovePlain(t *testing.T) {
	e, root := setup(t)
	src := filepath.Join(t.TempDir(), "app")
	os.WriteFile(src, []byte("binary"), 0o600)

	// A foreign file already at the destination is backed up.
	put(t, root, "/usr/local/bin/app", "manual install")
	rec, err := e.Place(src, "/usr/local/bin/app", 0o755, nil)
	if err != nil || rec.Backup != "/usr/local/bin/app.debforge-orig" || !rec.Plain {
		t.Fatalf("%+v %v", rec, err)
	}
	if fi, _ := os.Stat(root + "/usr/local/bin/app"); fi.Mode().Perm() != 0o755 {
		t.Fatal("mode")
	}
	// Reinstall over our own file: no new backup.
	os.WriteFile(src, []byte("binary v2"), 0o600)
	rec2, err := e.Place(src, "/usr/local/bin/app", 0o755, &rec)
	if err != nil || rec2.Backup != rec.Backup || get(t, root, "/usr/local/bin/app") != "binary v2" {
		t.Fatalf("%+v %v", rec2, err)
	}
	o, err := e.RemoveRecord("/usr/local/bin/app", rec2, "")
	if err != nil || o.Action != "restored" || get(t, root, "/usr/local/bin/app") != "manual install" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestSymlinkRecord(t *testing.T) {
	e, root := setup(t)
	rec, err := e.PlaceSymlink("../lib/x/run", "/usr/local/bin/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := os.Readlink(root + "/usr/local/bin/x"); l != "../lib/x/run" {
		t.Fatal(l)
	}
	o, err := e.RemoveRecord("/usr/local/bin/x", rec, "")
	if err != nil || o.Action != "deleted" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestRemoveRecordKeepsReplacedPlain(t *testing.T) {
	e, root := setup(t)
	src := filepath.Join(t.TempDir(), "app")
	os.WriteFile(src, []byte("ours"), 0o600)
	rec, _ := e.Place(src, "/opt/app/bin", 0o755, nil)
	put(t, root, "/opt/app/bin", "someone replaced it")
	o, _ := e.RemoveRecord("/opt/app/bin", rec, "")
	if o.Action != "kept" {
		t.Fatalf("%+v", o)
	}
}
