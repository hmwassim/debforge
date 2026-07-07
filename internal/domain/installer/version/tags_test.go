package version

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- RepoFromURL ---------------------------------------------------------

func TestRepoFromURL_github(t *testing.T) {
	repo, ok := RepoFromURL("https://github.com/owner/project/releases/download/v1.2.3/asset.deb")
	if !ok {
		t.Fatal("expected ok=true for a github.com URL")
	}
	if repo != "https://github.com/owner/project.git" {
		t.Errorf("got %q", repo)
	}
}

func TestRepoFromURL_gitlab(t *testing.T) {
	repo, ok := RepoFromURL("https://gitlab.com/owner/project/-/releases/v1.0.0/downloads/asset.tar.gz")
	if !ok {
		t.Fatal("expected ok=true for a gitlab.com URL")
	}
	if repo != "https://gitlab.com/owner/project.git" {
		t.Errorf("got %q", repo)
	}
}

func TestRepoFromURL_unsupportedHost(t *testing.T) {
	if _, ok := RepoFromURL("https://example.com/owner/project/asset.deb"); ok {
		t.Error("expected ok=false for an unsupported host")
	}
}

func TestRepoFromURL_tooShortPath(t *testing.T) {
	if _, ok := RepoFromURL("https://github.com/onlyowner"); ok {
		t.Error("expected ok=false when the path has fewer than owner/repo segments")
	}
}

func TestRepoFromURL_empty(t *testing.T) {
	if _, ok := RepoFromURL(""); ok {
		t.Error("expected ok=false for an empty URL")
	}
}

// ---- RepoFromPkg ----------------------------------------------------------

func TestRepoFromPkg_explicitRepoWins(t *testing.T) {
	p := &pkg.Package{
		Repo: "https://example.com/explicit.git",
		URLs: []string{"https://github.com/owner/project/releases/download/v1.0.0/asset.deb"},
	}
	if got := RepoFromPkg(p); got != "https://example.com/explicit.git" {
		t.Errorf("expected explicit Repo to take priority, got %q", got)
	}
}

func TestRepoFromPkg_derivedFromURL(t *testing.T) {
	p := &pkg.Package{URLs: []string{"https://github.com/owner/project/releases/download/v1.0.0/asset.deb"}}
	if got := RepoFromPkg(p); got != "https://github.com/owner/project.git" {
		t.Errorf("got %q", got)
	}
}

func TestRepoFromPkg_neitherSet(t *testing.T) {
	if got := RepoFromPkg(&pkg.Package{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---- LatestTag ------------------------------------------------------------

func runnerReturning(out []byte, err error) *testutil.MockRunner {
	return &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return out, nil, err
		},
	}
}

func TestLatestTag_picksHighestVersionNumerically(t *testing.T) {
	out := []byte(
		"abc\trefs/tags/v1.2.0\n" +
			"def\trefs/tags/v1.9.0\n" +
			"ghi\trefs/tags/v1.10.0\n", // numeric sort: 1.10.0 > 1.9.0, not lexical
	)
	got, err := LatestTag(context.Background(), runnerReturning(out, nil), "https://github.com/o/p.git", "", "")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "1.10.0" {
		t.Errorf("expected 1.10.0, got %q", got)
	}
}

func TestLatestTag_customPrefix(t *testing.T) {
	out := []byte("abc\trefs/tags/release-2.0\ndef\trefs/tags/release-1.0\n")
	got, err := LatestTag(context.Background(), runnerReturning(out, nil), "https://github.com/o/p.git", "release-", "")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "2.0" {
		t.Errorf("expected 2.0, got %q", got)
	}
}

func TestLatestTag_skipsNonNumericAndPeeledTags(t *testing.T) {
	out := []byte(
		"abc\trefs/tags/v1.0.0\n" +
			"abc\trefs/tags/v1.0.0^{}\n" + // peeled annotated tag, same version
			"def\trefs/tags/latest\n" + // non-numeric, should be skipped
			"ghi\trefs/heads/main\n", // not a tag ref at all
	)
	got, err := LatestTag(context.Background(), runnerReturning(out, nil), "https://github.com/o/p.git", "", "")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "1.0.0" {
		t.Errorf("expected 1.0.0, got %q", got)
	}
}

func TestLatestTag_noTagsFound(t *testing.T) {
	if _, err := LatestTag(context.Background(), runnerReturning([]byte(""), nil), "https://github.com/o/p.git", "", ""); err == nil {
		t.Error("expected an error when no tags are found")
	}
}

func TestLatestTag_runnerError(t *testing.T) {
	if _, err := LatestTag(context.Background(), runnerReturning(nil, errors.New("ls-remote failed")), "https://github.com/o/p.git", "", ""); err == nil {
		t.Error("expected an error when the runner fails")
	}
}

func TestLatestTag_verifyURLSkipsMissingAssets(t *testing.T) {
	// v2.0.0's asset 404s, v1.0.0's exists - LatestTag should fall back
	// to the highest tag that actually has a downloadable asset.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2.0.0.deb" { // tag v2.0.0 has its leading "v" stripped before substitution
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := []byte("abc\trefs/tags/v1.0.0\ndef\trefs/tags/v2.0.0\n")
	got, err := LatestTag(context.Background(), runnerReturning(out, nil), "https://github.com/o/p.git", "", srv.URL+"/{version}.deb")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "1.0.0" {
		t.Errorf("expected fallback to 1.0.0 when 2.0.0's asset 404s, got %q", got)
	}
}
