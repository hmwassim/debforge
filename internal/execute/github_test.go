package execute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hmwassim/debforge/internal/ui"
)

func TestGitHubSHA256(t *testing.T) {
	const sum = "862a717502f0b9601f23e849a4b4dfe3d69aed7e7ca262cf1bdc4a553f8de7ec"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/a/b/releases/tags/v1" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"assets":[{"name":"app.AppImage","digest":"sha256:` + sum + `"}]}`))
	}))
	defer srv.Close()
	x := &Exec{HTTP: srv.Client(), GitHubAPI: srv.URL, UI: ui.New(ui.Options{Out: io.Discard, Stdout: io.Discard}), Log: ui.OpenLog("")}
	ctx := context.Background()
	if got := x.githubSHA256(ctx, "https://github.com/a/b/releases/download/v1/app.AppImage"); got != sum {
		t.Errorf("published digest: got %q", got)
	}
	// A failed lookup leaves the download to HTTPS alone instead of failing it.
	if got := x.githubSHA256(ctx, "https://github.com/a/gone/releases/download/v1/app.AppImage"); got != "" {
		t.Errorf("failed lookup: got %q", got)
	}
	if got := x.githubSHA256(ctx, "https://example.com/app.AppImage"); got != "" {
		t.Errorf("non-GitHub URL: got %q", got)
	}
}
