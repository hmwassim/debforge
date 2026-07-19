package textutil

import "testing"

func BenchmarkFormatSize(b *testing.B) {
	for _, v := range []int64{0, 512, 1500, 999999, 1500000, 2500000000} {
		b.Run(sizeName(v), func(b *testing.B) {
			var sink string
			for b.Loop() {
				sink = FormatSize(v)
			}
			_ = sink
		})
	}
}

func BenchmarkSanitizeVersion(b *testing.B) {
	for _, v := range []string{
		"1.2.3",
		"1.2.3-beta.1+build.42",
		"$(whoami).1.0",
		"1.2.3;rm -rf /",
	} {
		b.Run(v, func(b *testing.B) {
			var sink string
			for b.Loop() {
				sink = SanitizeVersion(v)
			}
			_ = sink
		})
	}
}

func BenchmarkExpandVersion(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = ExpandVersion("https://example.com/pkg-{version}.tar.gz", "1.2.3")
	}
	_ = sink
}

func BenchmarkSha256Hex(b *testing.B) {
	data := []byte("debforge package definition content for hashing")
	var sink string
	for b.Loop() {
		sink = Sha256Hex(data)
	}
	_ = sink
}

func BenchmarkSplitLines(b *testing.B) {
	s := "line1\nline2\nline3\nline4\nline5\n"
	var sink []string
	for b.Loop() {
		sink = SplitLines(s)
	}
	_ = sink
}

func BenchmarkUcFirst(b *testing.B) {
	var sink string
	for b.Loop() {
		sink = UcFirst("hello world")
	}
	_ = sink
}

func sizeName(v int64) string {
	switch {
	case v >= 1000000000:
		return "GB"
	case v >= 1000000:
		return "MB"
	case v >= 1000:
		return "KB"
	default:
		return "bytes"
	}
}
