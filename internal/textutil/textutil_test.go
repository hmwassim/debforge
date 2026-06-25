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
