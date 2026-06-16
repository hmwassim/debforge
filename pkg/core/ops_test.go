package core

import (
	"os"
	"testing"
)

func TestVerifySetup_EmptyPlan(t *testing.T) {
	if !verifySetup(plan{}) {
		t.Error("verifySetup({}) = false, want true")
	}
}

func TestVerifySetup_ConfigContentMatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	content := "test config"
	mode := os.FileMode(0644)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: content, mode: mode},
		},
	}
	if !verifySetup(p) {
		t.Error("verifySetup with matching file = false, want true")
	}
}

func TestVerifySetup_ConfigModeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	content := "test config"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: content, mode: 0600},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with wrong mode = true, want false")
	}
}

func TestVerifySetup_ConfigContentMismatch(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.conf"
	mode := os.FileMode(0644)
	if err := os.WriteFile(path, []byte("actual content"), mode); err != nil {
		t.Fatal(err)
	}

	p := plan{
		configs: []configCheck{
			{dest: path, content: "expected content", mode: mode},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with wrong content = true, want false")
	}
}

func TestVerifySetup_ConfigFileMissing(t *testing.T) {
	p := plan{
		configs: []configCheck{
			{dest: "/nonexistent/path.conf", content: "x", mode: 0644},
		},
	}
	if verifySetup(p) {
		t.Error("verifySetup with missing config = true, want false")
	}
}

func TestSetDiff(t *testing.T) {
	tests := []struct {
		name string
		prev []string
		cur  []string
		want []string
	}{
		{name: "nil prev", prev: nil, cur: []string{"a", "b"}, want: nil},
		{name: "empty prev", prev: []string{}, cur: []string{"a", "b"}, want: nil},
		{name: "no diff", prev: []string{"a", "b"}, cur: []string{"a", "b"}, want: nil},
		{name: "some removed", prev: []string{"a", "b", "c"}, cur: []string{"a"}, want: []string{"b", "c"}},
		{name: "all removed", prev: []string{"a", "b"}, cur: []string{}, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setDiff(tt.prev, tt.cur)
			if len(got) != len(tt.want) {
				t.Fatalf("setDiff(%v, %v) = %v, want %v", tt.prev, tt.cur, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("setDiff(%v, %v) = %v, want %v", tt.prev, tt.cur, got, tt.want)
				}
			}
		})
	}
}
