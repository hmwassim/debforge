package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s := Store{Path: p}
	st, warn, err := s.Load()
	if err != nil || warn != nil || len(st.Packages) != 0 {
		t.Fatalf("empty load: %v %v", warn, err)
	}
	st.Packages["firefox"] = &Package{Kind: "apt", Explicit: true, AptPackages: []string{"firefox"},
		Files: map[string]File{"/etc/x": {SHA256: "ab", Mode: 0o644}}}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", fi.Mode())
	}
	st2, warn, err := s.Load()
	if err != nil || warn != nil {
		t.Fatal(warn, err)
	}
	if st2.Packages["firefox"].Files["/etc/x"].SHA256 != "ab" {
		t.Fatalf("round trip lost data: %+v", st2.Packages["firefox"])
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestCorruptMovedAside(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	os.WriteFile(p, []byte("{not json"), 0o644)
	s := Store{Path: p, Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }}
	st, warn, err := s.Load()
	if err != nil || warn == nil || st == nil {
		t.Fatalf("got %v %v", warn, err)
	}
	if _, err := os.Stat(p + ".corrupt-20260102-030405"); err != nil {
		t.Fatalf("not moved aside: %v", err)
	}
	if !strings.Contains(warn.Error(), "starting empty") {
		t.Fatal(warn)
	}
}

func TestOldSchemaMovedAside(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(p, []byte(`{"schema":1}`), 0o644)
	_, warn, err := Store{Path: p}.Load()
	if err != nil || warn == nil {
		t.Fatalf("got %v %v", warn, err)
	}
}

func TestClaims(t *testing.T) {
	st := New()
	st.Packages["a"] = &Package{AptPackages: []string{"x"}, Extrepos: []string{"r"}}
	st.Packages["b"] = &Package{AptPackages: []string{"x", "y"}}
	if !st.ClaimedApt("x", "a") || st.ClaimedApt("y", "b") {
		t.Fatal("ClaimedApt wrong")
	}
	if st.ClaimedExtrepo("r", "a") {
		t.Fatal("ClaimedExtrepo wrong")
	}
}
