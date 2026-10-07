package fetch

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hmwassim/debforge/internal/catalog"
	"github.com/hmwassim/debforge/internal/system"
)

// Resolver implements plan.Versions against the network.
type Resolver struct {
	R    system.Runner
	HTTP *http.Client
}

// Resolve returns the version to install for p.
func (r *Resolver) Resolve(ctx context.Context, p *catalog.Package) (string, error) {
	v := p.Version
	if v == nil {
		return "", nil
	}
	switch v.From {
	case "pin":
		return v.Pin, nil
	case "cmd":
		return FromCmd(ctx, r.R, v.Cmd)
	case "git-tags":
		versions, err := TagVersions(ctx, r.R, v.Repo, v.Tag, v.Prerelease)
		if err != nil {
			return "", err
		}
		asset := firstVersionedURL(p)
		var exists func(string) bool
		if asset != "" {
			exists = func(ver string) bool { return Exists(ctx, r.HTTP, Expand(asset, ver)) }
		}
		return Latest(versions, exists)
	}
	return "", fmt.Errorf("unknown version source %q", v.From)
}

func firstVersionedURL(p *catalog.Package) string {
	var urls []string
	switch p.Kind() {
	case catalog.KindDeb:
		for _, d := range p.Source.Deb.URLs {
			urls = append(urls, d.URL)
		}
	case catalog.KindArchive:
		urls = append(urls, p.Source.Archive.URL)
	case catalog.KindAppImage:
		urls = append(urls, p.Source.AppImage.URL)
	}
	for _, u := range urls {
		if strings.Contains(u, "{version}") {
			return u
		}
	}
	return ""
}
