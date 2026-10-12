package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/hmwassim/debforge/internal/catalog"
)

// GitHubAPI is the GitHub REST API base URL.
const GitHubAPI = "https://api.github.com"

// maxReleasesBody caps the releases API response.
const maxReleasesBody = 16 << 20

type ghRelease struct {
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/repos/"+slug+"/releases?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "debforge")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list releases of %s: %w", slug, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			msg := "GitHub API rate limit reached"
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				msg += " (resets at " + time.Unix(reset, 0).Format("15:04") + ")"
			}
			return nil, fmt.Errorf("list releases of %s: %s", slug, msg)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list releases of %s: HTTP %s", slug, resp.Status)
	}
	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleasesBody)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("list releases of %s: %w", slug, err)
	}
	return releases, nil
}
