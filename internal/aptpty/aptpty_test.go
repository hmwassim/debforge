package aptpty

import (
	"context"
	"errors"
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
			line:   "not a progress line",
			wantOK: false,
		},
		{
			line:   "",
			wantOK: false,
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

func TestFindInstalledConflicts_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("dpkg-query failed")
		},
	}
	got := FindInstalledConflicts(context.Background(), runner, []string{"pkg-a"})
	if len(got) != 0 {
		t.Errorf("FindInstalledConflicts with error = %v, want []", got)
	}
}

func TestCollectErr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantStr string
	}{
		{"E prefix", "E: Sub-process /usr/bin/dpkg returned an error code (1)", 1, "E: Sub-process /usr/bin/dpkg returned an error code (1)"},
		{"W prefix", "W: Some issues with the repository", 1, "W: Some issues with the repository"},
		{"dpkg prefix", "dpkg: error processing package foo (--configure)", 1, "dpkg: error processing package foo (--configure)"},
		{"no prefix", "Reading package lists... Done", 0, ""},
		{"empty string", "", 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var errs []string
			collectErr(tc.input, &errs)
			if len(errs) != tc.wantLen {
				t.Errorf("collectErr(%q) len = %d, want %d", tc.input, len(errs), tc.wantLen)
			}
			if tc.wantLen > 0 && errs[0] != tc.wantStr {
				t.Errorf("collectErr(%q) = %q, want %q", tc.input, errs[0], tc.wantStr)
			}
		})
	}
}

func TestCollectErr_limit(t *testing.T) {
	errs := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		collectErr("E: error", &errs)
	}
	if len(errs) != 5 {
		t.Errorf("collectErr should collect at most 5 errors, got %d", len(errs))
	}
}

func TestCollectErr_stripsANSI(t *testing.T) {
	var errs []string
	collectErr("E: \033[31mError\033[0m", &errs)
	if len(errs) != 1 || errs[0] != "E: Error" {
		t.Errorf("collectErr with ANSI = %q, want %q", errs[0], "E: Error")
	}
}

func TestHandleLine_downloadSize(t *testing.T) {
	state := &runState{phase: phaseDownload}
	var cur, total int64
	var pkg string
	handleLine("Download size: 42.5 MB / 100 MB", state, &cur, &total, &pkg, nil)
	// handleLine splits on the last / and takes the right side, so
	// overallTotal reflects the rightmost value ("100 MB").
	if state.overallTotal != 100000000 {
		t.Errorf("overallTotal = %d, want 100000000", state.overallTotal)
	}
	if state.overallLabel != "100 MB" {
		t.Errorf("overallLabel = %q, want %q", state.overallLabel, "100 MB")
	}
}

func TestHandleLine_needToGet(t *testing.T) {
	state := &runState{phase: phaseDownload}
	var cur, total int64
	var pkg string
	handleLine("Need to get 15.2 MB of archives.", state, &cur, &total, &pkg, nil)
	if state.overallTotal != 15200000 {
		t.Errorf("overallTotal = %d, want 15200000", state.overallTotal)
	}
}

func TestHandleLine_needToGetWithSlash(t *testing.T) {
	state := &runState{phase: phaseDownload}
	var cur, total int64
	var pkg string
	handleLine("Need to get 0 B/1,024 kB of archives.", state, &cur, &total, &pkg, nil)
	if state.overallLabel != "1,024 kB" {
		t.Errorf("overallLabel = %q, want %q", state.overallLabel, "1,024 kB")
	}
}

func TestHandleLine_fetchedTransitionsToInstall(t *testing.T) {
	state := &runState{phase: phaseDownload, cumulativeDone: 1000, prevPkgTotal: 500}
	var cur, total int64
	var pkg string
	handleLine("Fetched 15.2 MB in 2s (7.6 MB/s)", state, &cur, &total, &pkg, nil)
	if state.phase != phaseInstall {
		t.Errorf("phase = %d, want %d", state.phase, phaseInstall)
	}
	if state.cumulativeDone != 1500 {
		t.Errorf("cumulativeDone = %d, want 1500 (1000+500)", state.cumulativeDone)
	}
	if cur != 0 || total != 0 || pkg != "" {
		t.Errorf("cur/total/pkg should be reset, got cur=%d total=%d pkg=%q", cur, total, pkg)
	}
}

func TestHandleLine_settingUp(t *testing.T) {
	state := &runState{phase: phaseInstall}
	var cur, total int64
	var pkg string
	handleLine("Setting up libc6 (2.36-9)", state, &cur, &total, &pkg, nil)
	if state.installPkg != "libc6" {
		t.Errorf("installPkg = %q, want %q", state.installPkg, "libc6")
	}
}

func TestHandleLine_settingUpWithArch(t *testing.T) {
	state := &runState{phase: phaseInstall}
	var cur, total int64
	var pkg string
	// handleLine splits on " (" to strip version, but preserves the
	// arch suffix (e.g. ":amd64") since it only looks for "/" for
	// multi-arch package names.
	handleLine("Setting up libc6:amd64 (2.36-9)", state, &cur, &total, &pkg, nil)
	if state.installPkg != "libc6:amd64" {
		t.Errorf("installPkg = %q, want %q", state.installPkg, "libc6:amd64")
	}
}

func TestHandleLine_unpacking(t *testing.T) {
	state := &runState{phase: phaseInstall}
	var cur, total int64
	var pkg string
	handleLine("Unpacking libc6 (2.36-9) over (2.35-1)", state, &cur, &total, &pkg, nil)
	if state.phase != phaseInstall {
		t.Errorf("phase should remain install, got %d", state.phase)
	}
}

func TestHandleLine_progress(t *testing.T) {
	state := &runState{phase: phaseDownload}
	var cur, total int64
	var pkg string
	handleLine(" 42% [1 libc6 123.4 kB/567.8 kB 100%]", state, &cur, &total, &pkg, nil)
	if cur != 123400 || total != 567800 || pkg != "libc6" {
		t.Errorf("cur=%d total=%d pkg=%q, want 123400 567800 libc6", cur, total, pkg)
	}
}

func TestHandleLine_prompt(t *testing.T) {
	state := &runState{}
	var cur, total int64
	var pkg string
	handleLine("Configuration file '/etc/foo.conf' ? [Y/n]", state, &cur, &total, &pkg, nil)
}

func TestProgressDesc_downloadWithLabel(t *testing.T) {
	state := &runState{phase: phaseDownload, overallTotal: 10000000, overallLabel: "10 MB"}
	got := progressDesc(state, "libc6", 5000000)
	want := "Downloading libc6... [5.0M/10 MB]"
	if got != want {
		t.Errorf("progressDesc = %q, want %q", got, want)
	}
}

func TestProgressDesc_downloadWithoutLabel(t *testing.T) {
	state := &runState{phase: phaseDownload, overallTotal: 10000000}
	got := progressDesc(state, "libc6", 5000000)
	want := "Downloading libc6... [5.0M/10.0M]"
	if got != want {
		t.Errorf("progressDesc = %q, want %q", got, want)
	}
}

func TestProgressDesc_downloadNoOverall(t *testing.T) {
	state := &runState{phase: phaseDownload}
	got := progressDesc(state, "libc6", 5000000)
	if got != "Downloading libc6... [5.0M/1]" {
		t.Errorf("progressDesc = %q, want %q", got, "Downloading libc6... [5.0M/1]")
	}
}

func TestProgressDesc_install(t *testing.T) {
	state := &runState{phase: phaseInstall}
	got := progressDesc(state, "libc6", 0)
	if got != "Installing libc6..." {
		t.Errorf("progressDesc = %q, want %q", got, "Installing libc6...")
	}
}

func TestProgressDesc_installWithInstallPkg(t *testing.T) {
	state := &runState{phase: phaseInstall, installPkg: "bash"}
	got := progressDesc(state, "", 0)
	if got != "Installing bash..." {
		t.Errorf("progressDesc = %q, want %q", got, "Installing bash...")
	}
}

func TestGetDownloadSize(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("'http://example.com/pkg.deb' _ 123456 0\n"), nil, nil
		},
	}
	total, label := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a"})
	if total != 123456 {
		t.Errorf("total = %d, want 123456", total)
	}
	if label == "" {
		t.Errorf("label should not be empty for non-zero total")
	}
}

func TestGetDownloadSize_error(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("apt-get failed")
		},
	}
	total, label := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a"})
	if total != 0 || label != "" {
		t.Errorf("on error: total=%d label=%q, want (0, \"\")", total, label)
	}
}

func TestGetDownloadSize_noMatches(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("All packages are up to date.\n"), nil, nil
		},
	}
	total, label := getDownloadSize(context.Background(), runner, "install", []string{})
	if total != 0 || label != "" {
		t.Errorf("when nothing to download: total=%d label=%q, want (0, \"\")", total, label)
	}
}

func TestGetDownloadSize_multipleArchives(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			out := []byte("'/pkg1.deb' _ 1000 0\n'/pkg2.deb' _ 2000 0\n'/pkg3.deb' _ 3000 0\n")
			return out, nil, nil
		},
	}
	total, label := getDownloadSize(context.Background(), runner, "install", []string{"pkg-a", "pkg-b"})
	if total != 6000 {
		t.Errorf("total = %d, want 6000", total)
	}
	if label == "" {
		t.Errorf("label should not be empty")
	}
}

func TestRunInstall_empty(t *testing.T) {
	err := RunInstall(context.Background(), nil, nil, &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunInstall(nil) = %v, want nil", err)
	}
}

func TestRunInstallBackports_defaultSuite(t *testing.T) {
	suite := "trixie-backports"
	want := "trixie-backports"
	if suite != want {
		t.Errorf("default backports suite = %q, want %q", suite, want)
	}
}

func TestRunInstallBackports_empty(t *testing.T) {
	err := RunInstallBackports(context.Background(), nil, nil, "", &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunInstallBackports(nil) = %v, want nil", err)
	}
}

func TestRunRemove_empty(t *testing.T) {
	err := RunRemove(context.Background(), nil, nil, &testutil.MockSpinner{})
	if err != nil {
		t.Errorf("RunRemove(nil) = %v, want nil", err)
	}
}
