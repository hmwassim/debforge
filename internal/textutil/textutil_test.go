package textutil

import "testing"

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1k"},
		{999999, "999k"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
		{999999999, "1000.0M"},
		{1000000000, "1.0G"},
		{2500000000, "2.5G"},
	}
	for _, c := range cases {
		if got := FormatSize(c.in); got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUcFirst(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"hello", "Hello"},
		{"Hello", "Hello"},
		{"über", "Über"}, // non-ASCII rune, exercises the []rune path
		{"1abc", "1abc"}, // no letter to uppercase at position 0
	}
	for _, c := range cases {
		if got := UcFirst(c.in); got != c.want {
			t.Errorf("UcFirst(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"hello", []string{"hello"}},
		{"hello\nworld", []string{"hello", "world"}},
		{"hello\nworld\n", []string{"hello", "world"}},
		{"\n", []string{""}},
	}
	for _, c := range cases {
		got := SplitLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("SplitLines(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("SplitLines(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestExpandVersion(t *testing.T) {
	cases := []struct{ template, version, want string }{
		{"https://example.com/pkg_{version}.deb", "1.2.3", "https://example.com/pkg_1.2.3.deb"},
		{"no placeholder", "1.0", "no placeholder"},
		{"https://example.com/pkg_{version}/file-{version}.tar.gz", "v2.0", "https://example.com/pkg_v2.0/file-v2.0.tar.gz"},
		{"", "1.0", ""},
	}
	for _, c := range cases {
		if got := ExpandVersion(c.template, c.version); got != c.want {
			t.Errorf("ExpandVersion(%q, %q) = %q, want %q", c.template, c.version, got, c.want)
		}
	}
}
