package version

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkSelectTag(b *testing.B) {
	refs := make([]string, 50)
	for i := range refs {
		refs[i] = fmt.Sprintf("refs/tags/v1.%d.0", i)
	}
	b.ResetTimer()
	var sink string
	for b.Loop() {
		sink, _ = SelectTag(nil, refs, "https://github.com/user/repo", "v", "")
	}
	_ = sink
}

func BenchmarkSelectTag_withPrefix(b *testing.B) {
	refs := make([]string, 30)
	for i := range refs {
		refs[i] = fmt.Sprintf("refs/tags/pkg-%d.0.0", i)
	}
	b.ResetTimer()
	var sink string
	for b.Loop() {
		sink, _ = SelectTag(nil, refs, "https://github.com/user/repo", "pkg-", "")
	}
	_ = sink
}

func BenchmarkVersionLess(b *testing.B) {
	var sink bool
	for b.Loop() {
		sink = versionLess("1.9", "1.10")
	}
	_ = sink
}

func BenchmarkParseNums(b *testing.B) {
	var sink []int
	for b.Loop() {
		sink = parseNums("1.2.3-beta.42")
	}
	_ = sink
}

func BenchmarkRepoFromURL(b *testing.B) {
	for _, u := range []string{
		"https://github.com/user/repo/releases/download/v1.0/pkg.deb",
		"https://gitlab.com/user/repo/-/releases/v2.0/downloads/pkg.tar.gz",
		"https://example.com/file.deb",
	} {
		host, _, _ := strings.Cut(strings.TrimPrefix(u, "https://"), "/")
		b.Run(host, func(b *testing.B) {
			var sink string
			var sink2 bool
			for b.Loop() {
				sink, sink2 = RepoFromURL(u)
			}
			_, _ = sink, sink2
		})
	}
}
