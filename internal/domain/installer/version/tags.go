package version

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// RepoFromURL extracts a git repository URL from a release download URL for
// well-known hosts (GitHub, GitLab). Returns ("", false) if the host is
// unsupported or the URL cannot be parsed.
func RepoFromURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	if len(parts) < 2 {
		return "", false
	}
	switch u.Host {
	case "github.com":
		return fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]), true
	case "gitlab.com":
		return fmt.Sprintf("https://gitlab.com/%s/%s.git", parts[0], parts[1]), true
	}
	return "", false
}

// FetchTagRefs runs `git ls-remote --tags repoURL` and returns the raw
// "refs/tags/..." ref names, unfiltered by any package-specific prefix or
// verification. This is the expensive, network-bound half of version
// checking, and its result is identical for every package that shares a
// repo — callers that check multiple packages against the same repo
// (e.g. a family of source packages built from one monorepo) should cache
// this by repoURL rather than re-fetching per package.
//
// Do NOT cache the result of SelectTag the same way: TagPrefix and
// verifyURL are per-package and can legitimately produce a different
// answer from the same ref list (see SelectTag).
func FetchTagRefs(ctx context.Context, runner ports.CommandRunner, repoURL string) ([]string, error) {
	out, _, err := runner.Run(ctx, "git", "ls-remote", "--tags", repoURL)
	if err != nil {
		return nil, fmt.Errorf("ls-remote %s: %w", repoURL, err)
	}
	var refs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		refs = append(refs, parts[1])
	}
	return refs, nil
}

// SelectTag filters refs down to version tags matching prefix (defaults to
// "v" when empty), sorts them numerically (1.9 < 1.10), and returns the
// newest one. Only tags starting with a digit (after stripping the prefix)
// are considered.
//
// When verifyURL is non-empty, each candidate tag is verified by issuing a
// HEAD request to the URL template (with {version} substituted), walking
// from newest to oldest. Tags whose URL returns 404 are skipped, so only
// tags with actual release assets are accepted. This avoids relying on the
// GitHub API for release existence, and is why this step cannot be shared
// across packages that pull different assets from the same repo (each
// package's verifyURL can accept or reject a different tag).
func SelectTag(ctx context.Context, refs []string, repoURL, prefix, verifyURL string) (string, error) {
	if prefix == "" {
		prefix = "v"
	}

	var tags []string
	for _, ref := range refs {
		tag, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		tag = strings.TrimSuffix(tag, "^{}")

		cleaned := strings.TrimPrefix(tag, prefix)
		if cleaned == "" || cleaned[0] < '0' || cleaned[0] > '9' {
			continue
		}

		tags = append(tags, tag)
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("no version tags found in %s", repoURL)
	}

	slices.SortFunc(tags, func(a, b string) int {
		if versionLess(strings.TrimPrefix(a, prefix), strings.TrimPrefix(b, prefix)) {
			return -1
		}
		return 1
	})

	if verifyURL == "" {
		return strings.TrimPrefix(tags[len(tags)-1], prefix), nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for i := len(tags) - 1; i >= 0; i-- {
		v := strings.TrimPrefix(tags[i], prefix)
		u := strings.ReplaceAll(verifyURL, "{version}", v)

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			return v, nil
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("no version tag with a valid download URL found in %s (last attempt: %w)", repoURL, lastErr)
	}
	return "", fmt.Errorf("no version tag with a valid download URL found in %s", repoURL)
}

// LatestTag returns the newest tag from a git repository, stripped of the
// given prefix. Convenience wrapper combining FetchTagRefs + SelectTag for
// callers that check a single package and have no reason to cache the raw
// ref list. Callers checking multiple packages against the same repo
// should call FetchTagRefs once and SelectTag per package instead (see
// their docs for why the two must not be collapsed into one cached call).
func LatestTag(ctx context.Context, runner ports.CommandRunner, repoURL, prefix, verifyURL string) (string, error) {
	refs, err := FetchTagRefs(ctx, runner, repoURL)
	if err != nil {
		return "", err
	}
	return SelectTag(ctx, refs, repoURL, prefix, verifyURL)
}

func versionLess(a, b string) bool {
	an := parseNums(a)
	bn := parseNums(b)
	for i := 0; i < len(an) && i < len(bn); i++ {
		if an[i] != bn[i] {
			return an[i] < bn[i]
		}
	}
	return len(an) < len(bn)
}

// RepoFromPkg returns the git repository URL for a package, preferring the
// explicit Repo field over deriving it from the download URL.
func RepoFromPkg(p *pkg.Package) string {
	if p.Repo != "" {
		return p.Repo
	}
	if len(p.URLs) == 0 {
		return ""
	}
	repo, _ := RepoFromURL(p.URLs[0])
	return repo
}

func parseNums(v string) []int {
	var parts []int
	cur := 0
	inDig := false
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			cur = cur*10 + int(ch-'0')
			inDig = true
		} else {
			if inDig {
				parts = append(parts, cur)
				cur = 0
				inDig = false
			}
		}
	}
	if inDig {
		parts = append(parts, cur)
	}
	return parts
}
