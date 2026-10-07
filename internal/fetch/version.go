package fetch

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/system"
)

var (
	// validVersion is the only shape a version may have before it is
	// substituted into URLs or passed to hooks.
	validVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+~_-]{0,63}$`)
	digitRe      = regexp.MustCompile(`[0-9]+`)
	preMarkers   = regexp.MustCompile(`(?i)(alpha|beta|rc|pre|dev|nightly|snapshot)`)
)

// ValidVersion reports whether v is safe to interpolate.
func ValidVersion(v string) bool { return validVersion.MatchString(v) }

// Expand replaces {version} in s.
func Expand(s, version string) string { return strings.ReplaceAll(s, "{version}", version) }

// TagVersions lists versions from repo tags matching pattern (which
// contains {version}), newest first. Prereleases are dropped unless
// allowPre is set.
func TagVersions(ctx context.Context, r system.Runner, repo, pattern string, allowPre bool) ([]string, error) {
	res, err := r.Run(ctx, system.Cmd{
		Name: "git", Args: []string{"ls-remote", "--tags", "--refs", "--", repo},
	})
	if err != nil {
		return nil, fmt.Errorf("list tags of %s: %w", repo, err)
	}
	prefix, suffix, _ := strings.Cut(pattern, "{version}")
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if !strings.HasPrefix(tag, prefix) || !strings.HasSuffix(tag, suffix) || len(tag) <= len(prefix)+len(suffix) {
			continue
		}
		v := tag[len(prefix) : len(tag)-len(suffix)]
		if !ValidVersion(v) || seen[v] || v[0] < '0' || v[0] > '9' {
			continue
		}
		if !allowPre && preMarkers.MatchString(v) {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return Compare(out[i], out[j]) > 0 })
	return out, nil
}

// Compare orders versions by their numeric groups ("1.1-24" is
// 1,1,24). A prerelease (rc, beta, ...) sorts below the same release.
func Compare(a, b string) int {
	an, ap, apn := key(a)
	bn, bp, bpn := key(b)
	if c := cmpNums(an, bn); c != 0 {
		return c
	}
	switch {
	case ap == bp:
		return cmpNums(apn, bpn)
	case ap:
		return -1
	default:
		return 1
	}
}

func key(v string) (nums []int, pre bool, preNums []int) {
	base, rest := v, ""
	if loc := preMarkers.FindStringIndex(v); loc != nil {
		base, rest, pre = v[:loc[0]], v[loc[0]:], true
	}
	return digits(base), pre, digits(rest)
}

func digits(s string) []int {
	var out []int
	for _, g := range digitRe.FindAllString(s, -1) {
		n, _ := strconv.Atoi(g)
		out = append(out, n)
	}
	return out
}

func cmpNums(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// MaxProbes bounds how many candidate versions are checked for an asset.
const MaxProbes = 5

// Latest returns the newest version whose asset exists according to
// exists (which may be nil to accept the newest tag).
func Latest(versions []string, exists func(v string) bool) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no matching release tags found")
	}
	if exists == nil {
		return versions[0], nil
	}
	for i, v := range versions {
		if i >= MaxProbes {
			break
		}
		if exists(v) {
			return v, nil
		}
	}
	return "", fmt.Errorf("none of the newest %d releases (%s...) has a downloadable asset", min(len(versions), MaxProbes), versions[0])
}

// FromCmd runs a shell command and returns its trimmed output as the version.
func FromCmd(ctx context.Context, r system.Runner, cmd string) (string, error) {
	res, err := r.Run(ctx, system.Cmd{Name: "sh", Args: []string{"-eu", "-c", cmd}})
	if err != nil {
		return "", fmt.Errorf("version command: %w", err)
	}
	v := strings.TrimSpace(string(res.Stdout))
	if !ValidVersion(v) {
		return "", fmt.Errorf("version command printed invalid version %q", v)
	}
	return v, nil
}
