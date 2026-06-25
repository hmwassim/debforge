package version

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

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

// LatestTag returns the newest tag from a git repository, stripped of any
// leading "v". Only tags starting with a digit (after an optional "v") are
// considered. Tags are sorted by numeric version components (1.9 < 1.10).
func LatestTag(ctx context.Context, runner ports.CommandRunner, repoURL string) (string, error) {
	out, _, err := runner.Run(ctx, "git", "ls-remote", "--tags", repoURL)
	if err != nil {
		return "", fmt.Errorf("ls-remote %s: %w", repoURL, err)
	}

	var tags []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := parts[1]
		tag, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		tag = strings.TrimSuffix(tag, "^{}")

		cleaned := strings.TrimPrefix(tag, "v")
		if cleaned == "" || cleaned[0] < '0' || cleaned[0] > '9' {
			continue
		}

		tags = append(tags, tag)
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("no version tags found in %s", repoURL)
	}

	sort.Slice(tags, func(i, j int) bool {
		return versionLess(strings.TrimPrefix(tags[i], "v"), strings.TrimPrefix(tags[j], "v"))
	})

	return strings.TrimPrefix(tags[len(tags)-1], "v"), nil
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
	repo, _ := RepoFromURL(p.URL)
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
