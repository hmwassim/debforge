package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/system/systemtest"
)

func tlsServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *http.Client) {
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	c := NewClient()
	c.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	return srv, c
}

func TestDownloadChecksum(t *testing.T) {
	body := "hello world"
	sum := sha256.Sum256([]byte(body))
	srv, c := tlsServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	dest := filepath.Join(t.TempDir(), "x")
	if err := Download(context.Background(), c, srv.URL+"/f", dest, strings.ToUpper(hex.EncodeToString(sum[:])), nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != body {
		t.Fatal("content")
	}
	err := Download(context.Background(), c, srv.URL+"/f", dest+"2", strings.Repeat("0", 64), nil)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(dest + "2.part"); !os.IsNotExist(err) {
		t.Fatal(".part left behind")
	}
}

func TestDownloadRejectsHTTPRedirect(t *testing.T) {
	srv, c := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/evil.deb", http.StatusFound)
	})
	err := Download(context.Background(), c, srv.URL+"/f", filepath.Join(t.TempDir(), "x"), "", nil)
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("err = %v", err)
	}
}

func TestExistsOnly2xx(t *testing.T) {
	srv, c := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
		case "/limited":
			w.WriteHeader(429)
		default:
			w.WriteHeader(404)
		}
	})
	if !Exists(context.Background(), c, srv.URL+"/ok") || Exists(context.Background(), c, srv.URL+"/limited") || Exists(context.Background(), c, srv.URL+"/x") {
		t.Fatal("Exists wrong")
	}
}

const lsRemote = "a\trefs/tags/v1.9.5\n" +
	"g\trefs/tags/v1.9.6-2\n" +
	"b\trefs/tags/v2.0.0-rc1\n" +
	"c\trefs/tags/v1.10.0\n" +
	"d\trefs/tags/1.11.0\n" +
	"e\trefs/tags/v1.10.0.beta2\n" +
	"f\trefs/tags/nightly\n"

func TestTagVersions(t *testing.T) {
	r := (&systemtest.Runner{}).OK("git ls-remote", lsRemote)
	got, err := TagVersions(context.Background(), r, "https://github.com/a/b", "v{version}", false)
	if err != nil || strings.Join(got, ",") != "1.10.0,1.9.6-2,1.9.5" {
		t.Fatalf("got %v %v", got, err)
	}
	got, _ = TagVersions(context.Background(), r, "https://github.com/a/b", "v{version}", true)
	if got[0] != "2.0.0-rc1" {
		t.Fatalf("prerelease allowed: %v", got)
	}
	got, _ = TagVersions(context.Background(), r, "https://github.com/a/b", "{version}", false)
	if strings.Join(got, ",") != "1.11.0" {
		t.Fatalf("unprefixed pattern: %v", got)
	}
}

func TestCompare(t *testing.T) {
	cases := [][2]string{{"1.0.0", "1.0.0-rc1"}, {"1.10", "1.9"}, {"2.0", "1.99.99"}, {"1.0.1", "1.0"},
		{"1.1-24", "1.1-9"}, {"2.4.2", "1.1-24"}, {"1.0.0-rc2", "1.0.0-rc1"}, {"3.4.14-linux1", "3.4.13-linux1"}}
	for _, c := range cases {
		if Compare(c[0], c[1]) <= 0 || Compare(c[1], c[0]) >= 0 {
			t.Errorf("%s should be > %s", c[0], c[1])
		}
	}
	if Compare("1.0", "1.0") != 0 {
		t.Error("equal")
	}
}

func TestLatestProbes(t *testing.T) {
	v, err := Latest([]string{"3", "2", "1"}, func(v string) bool { return v == "2" })
	if err != nil || v != "2" {
		t.Fatalf("%s %v", v, err)
	}
	calls := 0
	_, err = Latest([]string{"9", "8", "7", "6", "5", "4", "3"}, func(string) bool { calls++; return false })
	if err == nil || calls != MaxProbes {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestValidVersion(t *testing.T) {
	for _, ok := range []string{"1.2.3", "1.0~rc1", "2024.01.02", "0.5.19+dfsg"} {
		if !ValidVersion(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "1;rm -rf /", "$(x)", "../1", "1 2", "1/2"} {
		if ValidVersion(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

// releasesJSON mirrors desktop-plus: plain-number tags, with betas and RCs
// only recognisable by their title or the prerelease flag.
const releasesJSON = `[
 {"tag_name": "v3.6.8.0", "name": "Desktop Plus v3.6.8", "draft": true},
 {"tag_name": "v3.6.7.1", "name": "Desktop Plus v3.6.7-beta2"},
 {"tag_name": "v3.6.7.0", "name": "Desktop Plus v3.6.7-beta2 - RC1"},
 {"tag_name": "v3.6.6.2", "name": "Desktop Plus v3.6.6 hotfix", "prerelease": true},
 {"tag_name": "v3.6.6.1", "name": "Desktop Plus v3.6.6"},
 {"tag_name": "v3.6.6.0", "name": "Desktop Plus v3.6.6-beta1"},
 {"tag_name": "v3.6.5.1", "name": "Desktop Plus v3.6.5"},
 {"tag_name": "worktree-release", "name": "Worktree"}
]`

func releasesServer(t *testing.T) (*httptest.Server, *http.Client) {
	return tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/a/b/releases":
			if r.URL.Query().Get("per_page") != "100" || r.Header.Get("Accept") != "application/vnd.github+json" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(releasesJSON))
		case "/repos/a/limited/releases":
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1700000000")
			w.WriteHeader(403)
		case "/dl/v3.6.6.1/app.deb", "/dl/v3.6.5.1/app.deb":
		default:
			w.WriteHeader(404)
		}
	})
}

func TestReleaseVersions(t *testing.T) {
	srv, c := releasesServer(t)
	got, err := ReleaseVersions(context.Background(), c, srv.URL, "https://github.com/a/b", "v{version}", false)
	if err != nil || strings.Join(got, ",") != "3.6.6.1,3.6.5.1" {
		t.Fatalf("got %v %v", got, err)
	}
	got, _ = ReleaseVersions(context.Background(), c, srv.URL, "https://github.com/a/b", "v{version}", true)
	if strings.Join(got, ",") != "3.6.7.1,3.6.7.0,3.6.6.2,3.6.6.1,3.6.6.0,3.6.5.1" {
		t.Fatalf("prerelease allowed (drafts never): %v", got)
	}
	_, err = ReleaseVersions(context.Background(), c, srv.URL, "https://github.com/a/limited", "v{version}", false)
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("err = %v", err)
	}
	_, err = ReleaseVersions(context.Background(), c, srv.URL, "https://github.com/a/missing", "v{version}", false)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveGitHubReleases(t *testing.T) {
	srv, c := releasesServer(t)
	p := &catalog.Package{
		Name:    "app",
		Version: &catalog.Version{From: "github-releases", Repo: "https://github.com/a/b", Tag: "v{version}"},
		Source:  catalog.Source{Deb: &catalog.DebSource{URLs: []catalog.Download{{URL: srv.URL + "/dl/v{version}/app.deb"}}}},
	}
	v, err := (&Resolver{HTTP: c, GitHubAPI: srv.URL}).Resolve(context.Background(), p)
	if err != nil || v != "3.6.6.1" {
		t.Fatalf("%q %v", v, err)
	}
}
