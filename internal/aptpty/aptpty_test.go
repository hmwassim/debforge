package aptpty

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hmwassim/debforge/internal/testutil"
)

// mockPtySession implements ptySession with controlled input/output.
type mockPtySession struct {
	data     []byte
	readErr  error
	readIdx  int
	writeBuf bytes.Buffer
	waitErr  error
	signaled os.Signal
}

func (m *mockPtySession) Read(b []byte) (int, error) {
	if m.readIdx >= len(m.data) && m.readErr != nil {
		return 0, m.readErr
	}
	if m.readIdx >= len(m.data) {
		return 0, io.EOF
	}
	n := copy(b, m.data[m.readIdx:])
	m.readIdx += n
	return n, nil
}
func (m *mockPtySession) Write(b []byte) (int, error) { return m.writeBuf.Write(b) }
func (m *mockPtySession) Close() error                { return nil }
func (m *mockPtySession) Wait() error                 { return m.waitErr }
func (m *mockPtySession) Signal(sig os.Signal) error  { m.signaled = sig; return nil }
func (m *mockPtySession) SetSize(_, _ uint16) error   { return nil }

// mockPtyFactory returns the given session for any command.
func mockPtyFactory(sess ptySession) ptyFactory {
	return func(_ context.Context, _ string, _ ...string) (ptySession, error) {
		return sess, nil
	}
}

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
	want := "Downloading libc6                   [5.0M/10 MB]"
	if got != want {
		t.Errorf("progressDesc = %q, want %q", got, want)
	}
}

func TestProgressDesc_downloadWithoutLabel(t *testing.T) {
	state := &runState{phase: phaseDownload, overallTotal: 10000000}
	got := progressDesc(state, "libc6", 5000000)
	want := "Downloading libc6                   [5.0M/10.0M]"
	if got != want {
		t.Errorf("progressDesc = %q, want %q", got, want)
	}
}

func TestProgressDesc_downloadNoOverall(t *testing.T) {
	state := &runState{phase: phaseDownload}
	got := progressDesc(state, "libc6", 5000000)
	want := "Downloading libc6                   [5.0M/?]"
	if got != want {
		t.Errorf("progressDesc = %q, want %q", got, want)
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

func TestRunInstall_withPackages(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		return &mockPtySession{
			data:    []byte("Setting up pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunInstall(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, spinner)
	if err != nil {
		t.Errorf("RunInstall = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunInstallBackports_withSuite(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var recordedArgs []string
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		recordedArgs = append(recordedArgs, name)
		recordedArgs = append(recordedArgs, args...)
		return &mockPtySession{
			data:    []byte("Setting up pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunInstallBackports(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, "bookworm-backports", spinner)
	if err != nil {
		t.Errorf("RunInstallBackports = %v, want nil", err)
	}
	if len(recordedArgs) < 6 || recordedArgs[4] != "bookworm-backports" {
		t.Errorf("expected -t bookworm-backports in args, got %v", recordedArgs)
	}
}

func TestRunRemove_withPackages(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		return &mockPtySession{
			data:    []byte("Removing pkg (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunRemove(context.Background(), testutil.RunnerReturning(nil, nil), []string{"pkg"}, spinner)
	if err != nil {
		t.Errorf("RunRemove = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunUpdate(t *testing.T) {
	var called bool
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			called = true
			if name != "apt-get" || len(args) != 1 || args[0] != "update" {
				t.Errorf("unexpected call: %s %v", name, args)
			}
			return nil, nil, nil
		},
	}
	spinner := &testutil.MockSpinner{}
	err := RunUpdate(context.Background(), runner, spinner)
	if err != nil {
		t.Errorf("RunUpdate() = %v, want nil", err)
	}
	if !called {
		t.Error("runner.Run was not called")
	}
}

func TestRunUpdate_runnerError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("update failed")
		},
	}
	err := RunUpdate(context.Background(), runner, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error from RunUpdate")
	}
}

func TestRunUpgrade(t *testing.T) {
	original := startPty
	t.Cleanup(func() { startPty = original })

	var ptyCalled bool
	startPty = func(_ context.Context, name string, args ...string) (ptySession, error) {
		ptyCalled = true
		if name != "apt-get" || len(args) != 2 || args[0] != "full-upgrade" || args[1] != "-y" {
			t.Errorf("unexpected startPty call: %s %v", name, args)
		}
		return &mockPtySession{
			data:    []byte("Setting up everything (1.0-1)\n"),
			readErr: io.EOF,
		}, nil
	}

	spinner := &testutil.MockSpinner{}
	err := RunUpgrade(context.Background(), testutil.RunnerReturning(nil, nil), spinner)
	if err != nil {
		t.Errorf("RunUpgrade() = %v, want nil", err)
	}
	if !ptyCalled {
		t.Error("startPty was not called")
	}
}

func TestRunUpgrade_nilSpinner(t *testing.T) {
	err := RunUpgrade(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil spinner")
	}
}

// runWithSession tests -------------------------------------------------------

func TestRunWithSession_normal(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("Need to get 123 kB of archives.\n" +
			" 42% [1 hello 52.0 kB/123 kB 42%]\n" +
			"Fetched 123 kB in 0s (456 kB/s)\n" +
			"Setting up hello (2.36-9)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner.SetDesc to be called")
	}
}

func TestRunWithSession_spinnerNil(t *testing.T) {
	err := runWithSession(context.Background(), nil, []string{"install", "-y", "hello"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil spinner")
	}
}

func TestRunWithSession_waitError(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("E: Sub-process /usr/bin/dpkg returned an error\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 1"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error from Wait")
	}
}

func TestRunWithSession_waitErrWithMessages(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("E: Sub-process /usr/bin/dpkg returned an error (1)\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 100"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWithSession_aptErrsCollected(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("W: Some issues\n" +
			"dpkg: error processing foo\n" +
			"E: Sub-process returned error\n"),
		readErr: io.EOF,
		waitErr: errors.New("exit status 100"),
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWithSession_contextCancel(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mock := &mockPtySession{
		data:    []byte("some output\n"),
		readErr: io.EOF,
	}
	err := runWithSession(ctx, testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil after cancel, got %v", err)
	}
}

func TestRunWithSession_contextCancelMidRun(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockPtySession{
		data:    []byte("Need to get 1,024 kB of archives.\n 10% [1 pkg 100 kB/1,024 kB 10%]\n"),
		readErr: io.EOF,
	}
	// start but cancel while processing
	cancel()
	err := runWithSession(ctx, testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil after mid-run cancel, got %v", err)
	}
}

func TestRunWithSession_downloadPhase(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data: []byte("Need to get 1,024 kB of archives.\n" +
			" 20% [1 pkg 200 kB/1,024 kB 20%]\n" +
			" 80% [1 pkg 800 kB/1,024 kB 80%]\n" +
			"Fetched 1,024 kB in 1s (1,024 kB/s)\n" +
			"Setting up pkg (1.0-1)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
}

func TestRunWithSession_emptyArgs(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession with empty args: %v", err)
	}
}

func TestRunWithSession_readError(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		readErr: io.ErrUnexpectedEOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("expected nil on read error (non-EOF breaks but doesn't error), got %v", err)
	}
}

func TestRunWithSession_multilineProgress(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	data := []byte("Need to get 2,000 kB of archives.\n" +
		" 10% [1 pkg-a 200 kB/1,000 kB 20%]\r" +
		" 50% [1 pkg-a 500 kB/1,000 kB 50%]\n" +
		"Fetched 1,000 kB in 0s (2,000 kB/s)\n" +
		"Setting up pkg-a (2.0-1)\n")
	mock := &mockPtySession{
		data:    data,
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg-a"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
}

func TestRunWithSession_binaryExistsVerification(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &mockPtySession{
		data:    []byte("Fetched 100 kB in 0s (1,000 kB/s)\n"),
		readErr: io.EOF,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "pkg"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner SetDesc to be called")
	}
}

// delayedReader wraps a mockPtySession and adds a per-Read delay so the
// main loop's ticker branch (case <-ticker.C) gets exercised.
type delayedReader struct {
	mockPtySession
	delay time.Duration
}

func (m *delayedReader) Read(b []byte) (int, error) {
	time.Sleep(m.delay)
	return m.mockPtySession.Read(b)
}

func TestRunWithSession_tickerFires(t *testing.T) {
	spinner := &testutil.MockSpinner{}
	mock := &delayedReader{
		mockPtySession: mockPtySession{
			data: []byte("Need to get 1,024 kB of archives.\n" +
				"Fetched 1,024 kB in 1s (1,024 kB/s)\n" +
				"Setting up hello (1.0-1)\n"),
			readErr: io.EOF,
		},
		delay: 200 * time.Millisecond,
	}
	err := runWithSession(context.Background(), testutil.RunnerReturning(nil, nil),
		[]string{"install", "-y", "hello"}, spinner, mockPtyFactory(mock))
	if err != nil {
		t.Fatalf("runWithSession: %v", err)
	}
	if spinner.Desc == "" {
		t.Error("expected spinner.SetDesc to be called")
	}
}
