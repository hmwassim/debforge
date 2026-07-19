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

func TestSanitizeVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean version", "1.2.3", "1.2.3"},
		{"with hyphens", "1.2.3-beta1", "1.2.3-beta1"},
		{"with underscores", "1.2.3_beta1", "1.2.3_beta1"},
		{"with plus", "1.2.3+build1", "1.2.3+build1"},
		{"shell injection $(cmd)", "$(rm -rf /)", "rm-rf"},
		{"shell injection backtick", "`rm -rf /`", "rm-rf"},
		{"shell injection semicolon", "1.0;rm -rf /", "1.0rm-rf"},
		{"shell injection pipe", "1.0|cat /etc/passwd", "1.0catetcpasswd"},
		{"shell injection ampersand", "1.0&malicious", "1.0malicious"},
		{"shell injection dollar", "$HOME", "HOME"},
		{"empty string", "", ""},
		{"spaces stripped", "1.0 beta", "1.0beta"},
		{"quotes stripped", `1.0"evil"`, "1.0evil"},
		{"unicode stripped", "1.0β", "1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeVersion(tt.in); got != tt.want {
				t.Errorf("SanitizeVersion(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
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
