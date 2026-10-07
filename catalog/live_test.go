package catalogdata

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/fetch"
	"github.com/hmwassim/debforge/internal/setup"
	"github.com/hmwassim/debforge/internal/system"
)

// TestLiveVersions resolves every package's upstream version over the
// network (tags, asset HEAD checks, version commands). Run with
// DEBFORGE_LIVE=1; skipped otherwise.
func TestLiveVersions(t *testing.T) {
	if os.Getenv("DEBFORGE_LIVE") == "" {
		t.Skip("set DEBFORGE_LIVE=1 to check upstream versions")
	}
	c, _, err := catalog.Load(catalog.Layer{Name: "embedded", FS: FS, PkgDir: "packages", FilesDir: "files"})
	if err != nil {
		t.Fatal(err)
	}
	r := &fetch.Resolver{R: system.ExecRunner{}, HTTP: fetch.NewClient()}
	for _, p := range c.All() {
		if p.Version == nil {
			continue
		}
		p := p
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			v, err := r.Resolve(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if !fetch.ValidVersion(v) {
				t.Fatalf("invalid version %q", v)
			}
			t.Logf("%s %s", p.Name, v)
		})
	}
}

// TestLiveAptNames checks that every Debian package name the catalog uses
// has an install candidate on this (Trixie, backports enabled) host.
// Packages from extrepo or third-party repos are skipped.
func TestLiveAptNames(t *testing.T) {
	if os.Getenv("DEBFORGE_LIVE") == "" {
		t.Skip("set DEBFORGE_LIVE=1 to check apt package names")
	}
	c, _, err := catalog.Load(catalog.Layer{Name: "embedded", FS: FS, PkgDir: "packages", FilesDir: "files"})
	if err != nil {
		t.Fatal(err)
	}
	r := system.ExecRunner{}
	prof, err := setup.Load(FS, "default")
	if err != nil {
		t.Fatal(err)
	}
	check := func(owner, n string) {
		res, err := r.Run(context.Background(), system.Cmd{Name: "apt-cache", Args: []string{"policy", n}})
		if err != nil || !strings.Contains(string(res.Stdout), "Candidate:") || strings.Contains(string(res.Stdout), "Candidate: (none)") {
			t.Errorf("%s: apt package %q has no candidate", owner, n)
		}
	}
	for _, s := range prof.Steps {
		for _, n := range s.Packages {
			check("setup step "+s.ID, n)
		}
	}
	for _, p := range c.All() {
		names := append([]string(nil), p.Requires...)
		if a := p.Source.Apt; a != nil && len(a.Extrepo) == 0 && a.Repo == nil {
			names = append(names, a.Packages...)
			names = append(names, a.Backports...)
		}
		for _, n := range names {
			check(p.Name, n)
		}
	}
}
