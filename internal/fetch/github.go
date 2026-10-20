package fetch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hmwassim/debforge/internal/catalog"
)

// GitHubAPI is the GitHub REST API base URL.
const GitHubAPI = "https://api.github.com"

// maxReleasesBody caps the releases API response.
const maxReleasesBody = 16 << 20

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Name       string    `json:"name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"` // "sha256:<hex>"; empty for assets uploaded before GitHub added digests
}

// releaseAsset matches a GitHub release download URL: owner, repo, tag, file.
var releaseAsset = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^/?#]+)$`)

// AssetSHA256 returns the sha256 GitHub publishes for a release asset URL.
// It returns "" without an error when rawURL is not a release asset or the
// asset has no digest.
func AssetSHA256(ctx context.Context, c *http.Client, api, rawURL string) (string, error) {
	m := releaseAsset.FindStringSubmatch(rawURL)
	if m == nil {
		return "", nil
	}
	tag, err := url.PathUnescape(m[3])
	if err != nil {
		return "", nil
	}
	name, err := url.PathUnescape(m[4])
	if err != nil {
		return "", nil
	}
	slug := m[1] + "/" + m[2]
	var rel ghRelease
	if err := githubGet(ctx, c, api+"/repos/"+slug+"/releases/tags/"+url.PathEscape(tag), "get release "+tag+" of "+slug, maxReleasesBody, &rel); err != nil {
		return "", err
	}
	for _, a := range rel.Assets {
		if a.Name != name {
			continue
		}
		d, ok := strings.CutPrefix(a.Digest, "sha256:")
		if _, err := hex.DecodeString(d); !ok || len(d) != 64 || err != nil {
			return "", nil
		}
		return d, nil
	}
	return "", nil
}

// ReleaseVersions lists versions from the newest 100 GitHub releases of repo
// whose tag matches pattern (which contains {version}), newest first.
// Drafts are always dropped. Unless allowPre is set, so are releases GitHub
// flags as prereleases and releases whose tag or title has a prerelease
// marker: some projects tag betas with plain numbers and only say "beta" in
// the title.
func ReleaseVersions(ctx context.Context, c *http.Client, api, repo, pattern string, allowPre bool) ([]string, error) {
	slug, err := catalog.GitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	releases, err := githubReleases(ctx, c, api, slug)
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, r := range releases {
		if r.Draft || (!allowPre && (r.Prerelease || IsPrerelease(r.Tag) || IsPrerelease(r.Name))) {
			continue
		}
		tags = append(tags, r.Tag)
	}
	return tagsToVersions(tags, pattern, allowPre), nil
}

func githubReleases(ctx context.Context, c *http.Client, api, slug string) ([]ghRelease, error) {
	var releases []ghRelease
	err := githubGet(ctx, c, api+"/repos/"+slug+"/releases?per_page=100", "list releases of "+slug, maxReleasesBody, &releases)
	return releases, err
}

// githubGet decodes the JSON answer of a GitHub API GET into v. what
// prefixes errors, e.g. "list releases of owner/repo".
func githubGet(ctx context.Context, c *http.Client, apiURL, what string, limit int64, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "debforge")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			msg := "GitHub API rate limit reached"
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				msg += " (resets at " + time.Unix(reset, 0).Format("15:04") + ")"
			}
			return fmt.Errorf("%s: %s", what, msg)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %s", what, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}
