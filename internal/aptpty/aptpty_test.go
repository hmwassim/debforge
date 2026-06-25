package aptpty

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		val  string
		unit string
		want int64
	}{
		{"100", "KB", 100000},
		{"1.5", "MB", 1500000},
		{"2", "GB", 2000000000},
		{"500", "bytes", 500},
		{"1,234", "KB", 1234000},
		{"0", "MB", 0},
	}
	for _, tc := range tests {
		got := parseSize(tc.val, tc.unit)
		if got != tc.want {
			t.Errorf("parseSize(%q, %q) = %d, want %d", tc.val, tc.unit, got, tc.want)
		}
	}
}

func TestParseProgress(t *testing.T) {
	tests := []struct {
		line      string
		wantCur   int64
		wantTotal int64
		wantPkg   string
		wantOK    bool
	}{
		{
			line:      " 42% [1 libc6 123.4 kB/567.8 kB 100%]",
			wantCur:   123400,
			wantTotal: 567800,
			wantPkg:   "libc6",
			wantOK:    true,
		},
		{
			line:      " 99% [2 foo 1.5 MB/2.0 MB 100%]",
			wantCur:   1500000,
			wantTotal: 2000000,
			wantPkg:   "foo",
			wantOK:    true,
		},
		{
			line:    "not a progress line",
			wantOK:  false,
		},
		{
			line:    "",
			wantOK:  false,
		},
	}
	for _, tc := range tests {
		cur, total, pkg, ok := parseProgress(tc.line)
		if ok != tc.wantOK {
			t.Errorf("parseProgress(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
		}
		if ok {
			if cur != tc.wantCur {
				t.Errorf("parseProgress(%q) cur = %d, want %d", tc.line, cur, tc.wantCur)
			}
			if total != tc.wantTotal {
				t.Errorf("parseProgress(%q) total = %d, want %d", tc.line, total, tc.wantTotal)
			}
			if pkg != tc.wantPkg {
				t.Errorf("parseProgress(%q) pkg = %q, want %q", tc.line, pkg, tc.wantPkg)
			}
		}
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"\033[31mred\033[0m", "red"},
		{"\033[1;32mbold green\033[0m", "bold green"},
		{"plain\033[A", "plain"},
		{"", ""},
	}
	for _, tc := range tests {
		got := stripANSI(tc.input)
		if got != tc.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFindInstalledConflicts(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if len(args) >= 3 && args[2] == "pkg-a" {
				return []byte("installed"), nil, nil
			}
			return []byte("not-installed"), nil, nil
		},
	}

	got := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b", "pkg-c"})
	if len(got) != 1 || got[0] != "pkg-a" {
		t.Errorf("FindInstalledConflicts = %v, want [pkg-a]", got)
	}
}

func TestFindInstalledConflicts_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("not-installed"), nil, nil
		},
	}

	got := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a", "pkg-b"})
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts = %v, want []", got)
	}
}

func TestFindInstalledConflicts_empty(t *testing.T) {
	runner := &testutil.MockRunner{}
	got := FindInstalledConflicts(context.Background(), runner, nil)
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts(nil) = %v, want []", got)
	}
}
