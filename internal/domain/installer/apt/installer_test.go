package apt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- candidateVersion / isUpToDate ----------------------------------------

func policyOutput(candidate string) []byte {
	return []byte("test-pkg:\n  Installed: 1.0.0\n  Candidate: " + candidate + "\n  Version table:\n")
}

func TestCandidateVersion_parsesPolicyOutput(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return policyOutput("2.0.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner}

	got, err := inst.candidateVersion(context.Background(), "test-pkg")
	if err != nil {
		t.Fatalf("candidateVersion: %v", err)
	}
	if got != "2.0.0" {
		t.Errorf("got %q, want %q", got, "2.0.0")
	}
}

func TestCandidateVersion_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return policyOutput("(none)"), nil, nil
		},
	}
	inst := &Installer{runner: runner}

	got, err := inst.candidateVersion(context.Background(), "unknown-pkg")
	if err != nil {
		t.Fatalf("candidateVersion: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty candidate for (none), got %q", got)
	}
}

func TestIsUpToDate_firstInstallRecordsVersionAndProceeds(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return policyOutput("1.5.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-pkg", Packages: []string{"test-pkg"}, Apt: &pkg.AptConfig{}}

	upToDate, err := inst.isUpToDate(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("isUpToDate: %v", err)
	}
	if upToDate {
		t.Error("expected first install to not be considered up to date")
	}
	if p.Version != "1.5.0" {
		t.Errorf("expected p.Version to be recorded as 1.5.0, got %q", p.Version)
	}
}

func TestIsUpToDate_unchangedCandidateShortCircuits(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return policyOutput("1.5.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-pkg", Packages: []string{"test-pkg"}, Version: "1.5.0", Apt: &pkg.AptConfig{}}

	upToDate, err := inst.isUpToDate(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("isUpToDate: %v", err)
	}
	if !upToDate {
		t.Error("expected isUpToDate=true when candidate matches recorded version")
	}
}

func TestIsUpToDate_newCandidateProceedsAndUpdatesVersion(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return policyOutput("2.0.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-pkg", Packages: []string{"test-pkg"}, Version: "1.5.0", Apt: &pkg.AptConfig{}}

	upToDate, err := inst.isUpToDate(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("isUpToDate: %v", err)
	}
	if upToDate {
		t.Error("expected isUpToDate=false when a newer candidate is available")
	}
	if p.Version != "2.0.0" {
		t.Errorf("expected p.Version updated to 2.0.0, got %q", p.Version)
	}
}

// ---- SelectVariant ---------------------------------------------------------

func TestSelectVariant_yesModeDefaultsToSkip(t *testing.T) {
	ui := &testutil.MockUI{Yes: true}
	inst := &Installer{ui: ui}
	p := &pkg.Package{
		Name: "test-pkg",
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"zeta": {"pkg-zeta"}, "alpha": {"pkg-alpha"}},
		},
	}

	if err := inst.SelectVariant(context.Background(), p); err != nil {
		t.Fatalf("selectVariant: %v", err)
	}
	if p.Apt.Variant != "__skip__" {
		t.Errorf("expected yes-mode to default to skip, got %q", p.Apt.Variant)
	}
}

func TestSelectVariant_interactiveUsesPromptedValue(t *testing.T) {
	ui := &testutil.MockUI{
		PromptInputFunc: func(defaultVal, format string, args ...any) string {
			return "staging"
		},
	}
	inst := &Installer{ui: ui}
	p := &pkg.Package{
		Name: "test-pkg",
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}, "staging": {"pkg-staging"}},
		},
	}

	if err := inst.SelectVariant(context.Background(), p); err != nil {
		t.Fatalf("selectVariant: %v", err)
	}
	if p.Apt.Variant != "staging" {
		t.Errorf("expected prompted variant %q, got %q", "staging", p.Apt.Variant)
	}
}

func TestSelectVariant_invalidInputErrors(t *testing.T) {
	ui := &testutil.MockUI{
		PromptInputFunc: func(defaultVal, format string, args ...any) string {
			return "not-a-real-variant"
		},
	}
	inst := &Installer{ui: ui}
	p := &pkg.Package{
		Name: "test-pkg",
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}},
		},
	}

	if err := inst.SelectVariant(context.Background(), p); err == nil {
		t.Error("expected an error for an invalid variant selection")
	}
}

func TestSelectVariant_noopWhenAlreadySelected(t *testing.T) {
	ui := &testutil.MockUI{
		PromptInputFunc: func(defaultVal, format string, args ...any) string {
			t.Fatal("should not prompt when a variant is already selected")
			return ""
		},
	}
	inst := &Installer{ui: ui}
	p := &pkg.Package{
		Name: "test-pkg",
		Apt: &pkg.AptConfig{
			Variants: map[string][]string{"stable": {"pkg-stable"}},
			Variant:  "stable",
		},
	}

	if err := inst.SelectVariant(context.Background(), p); err != nil {
		t.Fatalf("selectVariant: %v", err)
	}
}

func TestSelectVariant_noopWhenNoVariants(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.SelectVariant(context.Background(), p); err != nil {
		t.Fatalf("selectVariant: %v", err)
	}
}

// ---- CheckGPU ---------------------------------------------------------------

func TestCheckGPU_nvidiaDetected(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return []byte("VGA compatible controller: NVIDIA Corporation GP106 [GeForce GTX 1060 6GB]"), nil, nil
		},
	}
	if err := CheckGPU(context.Background(), runner, "nvidia"); err != nil {
		t.Fatalf("CheckGPU: %v", err)
	}
}

func TestCheckGPU_noGPU(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return []byte("VGA compatible controller: Intel Corporation HD Graphics 620"), nil, nil
		},
	}
	if err := CheckGPU(context.Background(), runner, "nvidia"); err == nil {
		t.Fatal("expected error for missing NVIDIA GPU")
	}
}

func TestCheckGPU_unrelatedPackage(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("lspci should not be called for non-nvidia packages")
			return nil, nil, nil
		},
	}
	if err := CheckGPU(context.Background(), runner, "firefox"); err != nil {
		t.Fatalf("CheckGPU: %v", err)
	}
}

func TestCheckGPU_lspciError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("lspci not found")
		},
	}
	if err := CheckGPU(context.Background(), runner, "nvidia"); err == nil {
		t.Fatal("expected error from lspci failure")
	}
}

// ---- checkConflicts ---------------------------------------------------------

func TestCheckConflicts_removesFound(t *testing.T) {
	var gotArgs []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed"), nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Conflicts: []string{"conflict-pkg"}}}

	if err := inst.checkConflicts(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkConflicts: %v", err)
	}
	if len(gotArgs) == 0 {
		t.Fatal("expected execApt to be called")
	}
	if gotArgs[0] != "remove" || gotArgs[1] != "-y" || gotArgs[2] != "conflict-pkg" {
		t.Errorf("unexpected args: %v", gotArgs)
	}
}

func TestCheckConflicts_none(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.checkConflicts(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("checkConflicts: %v", err)
	}
}

// ---- enableExtrepos ---------------------------------------------------------

func TestEnableExtrepos_enablesAndUpdates(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{runner: runner, fs: fs}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	want := []string{"extrepo enable myrepo", "apt-get update"}
	for i, c := range want {
		if i >= len(calls) || calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestEnableExtrepos_noRepos(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands when no extrepos")
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
}

func TestEnableExtrepos_alreadyEnabled(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name)
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/apt/sources.list.d/extrepo_myrepo.sources"] = []byte("Enabled: yes\n")
	inst := &Installer{runner: runner, fs: fs}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Extrepo: []string{"myrepo"}}}

	if err := inst.enableExtrepos(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("enableExtrepos: %v", err)
	}
	if len(calls) != 1 || calls[0] != "apt-get" {
		t.Errorf("expected only apt-get update, got %v", calls)
	}
}

// ---- Install / Remove integration (via execApt) -----------------------------

func TestInstallMain_callsExecApt(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Packages: []string{"pkg-a", "pkg-b"}, Apt: &pkg.AptConfig{}}

	if err := inst.installMain(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installMain: %v", err)
	}
	want := []string{"install", "-y", "pkg-a", "pkg-b"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestInstallMain_emptySkips(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called with no packages")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Packages: nil, Apt: &pkg.AptConfig{}}

	if err := inst.installMain(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installMain: %v", err)
	}
}

func TestInstallBackports_callsExecApt(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Backports: []string{"bpkg"}, BackportSuite: "bookworm-backports"}}

	if err := inst.installBackports(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installBackports: %v", err)
	}
	want := []string{"install", "-y", "-t", "bookworm-backports", "bpkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestInstallBackports_defaultSuite(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Backports: []string{"bpkg"}}} // no BackportSuite

	if err := inst.installBackports(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installBackports: %v", err)
	}
	want := []string{"install", "-y", "-t", "trixie-backports", "bpkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestInstallBackports_emptySkips(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called with no backports")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{}}

	if err := inst.installBackports(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installBackports: %v", err)
	}
}

func TestRemove_callsExecApt(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Packages: []string{"pkg-a"}, Apt: &pkg.AptConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "pkg-a"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_emptySkips(t *testing.T) {
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called with no packages")
			return nil
		},
	}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestRemove_usesRemoveField(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Remove:   []string{"rm-pkg"},
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "rm-pkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test-pkg", Type: pkg.TypeDeb}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

// ---- NewInstaller -----------------------------------------------------------

func TestNewInstaller(t *testing.T) {
	runner := &testutil.MockRunner{}
	fs := testutil.NewMockFileSystem()
	ui := &testutil.MockUI{}
	inst := NewInstaller(runner, fs, ui)
	if inst.runner != runner {
		t.Error("runner not set")
	}
	if inst.fs != fs {
		t.Error("fs not set")
	}
	if inst.ui != ui {
		t.Error("ui not set")
	}
	if inst.execApt == nil {
		t.Error("execApt should not be nil")
	}
}

// ---- checkGPU ---------------------------------------------------------------

func TestCheckGPU_instanceCallsCheckGPU(t *testing.T) {
	var called bool
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			called = true
			return []byte("NVIDIA Corporation"), nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "nvidia"}
	if err := inst.checkGPU(context.Background(), p); err != nil {
		t.Fatalf("checkGPU: %v", err)
	}
	if !called {
		t.Error("runner.Run was not called")
	}
}

// ---- disableExtrepos --------------------------------------------------------

func TestDisableExtrepos_disablesEachRepo(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{Extrepo: []string{"repo-a"}}}

	inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{})

	want := []string{"extrepo disable repo-a"}
	for i, c := range want {
		if i >= len(calls) || calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestDisableExtrepos_multipleRepos(t *testing.T) {
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{Extrepo: []string{"repo-a", "repo-b"}}}

	inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{})

	want := []string{"extrepo disable repo-a", "extrepo disable repo-b"}
	if len(calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %v", len(calls), len(want), calls)
	}
	for i, c := range want {
		if calls[i] != c {
			t.Errorf("call %d: got %q, want %q", i, calls[i], c)
		}
	}
}

func TestDisableExtrepos_noRepos(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			t.Fatal("should not call any commands")
			return nil, nil, nil
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{Name: "test", Apt: &pkg.AptConfig{}}

	inst.disableExtrepos(context.Background(), p, &testutil.MockSpinner{})
}

// ---- writeConfigs -----------------------------------------------------------

func TestWriteConfigs_writes(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	inst := &Installer{fs: fs}
	p := &pkg.Package{Name: "test", Configs: map[string]string{
		"/etc/test/config": "value",
	}}

	if err := inst.writeConfigs(p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("writeConfigs: %v", err)
	}
	data, ok := fs.Files["/etc/test/config"]
	if !ok {
		t.Fatal("config file not written")
	}
	if string(data) != "value" {
		t.Errorf("got %q, want %q", string(data), "value")
	}
}

func TestWriteConfigs_empty(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	inst := &Installer{fs: fs}
	p := &pkg.Package{Name: "test"}

	if err := inst.writeConfigs(p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("writeConfigs: %v", err)
	}
}

// ---- Install ---------------------------------------------------------------

func TestInstall_wrongType(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeDeb}
	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestInstall_noPackagesNoVariants(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{Name: "test", Type: pkg.TypeApt, Apt: &pkg.AptConfig{}}
	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error when no packages or variants")
	}
}

func TestInstall_skipVariantShortCircuits(t *testing.T) {
	inst := &Installer{}
	p := &pkg.Package{
		Name:     "test",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Apt:      &pkg.AptConfig{Variant: skipVariant},
	}
	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
}

func TestInstallMain_withVariant(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Packages: []string{"base-pkg"},
		Apt:      &pkg.AptConfig{Variant: "pro", Variants: map[string][]string{"pro": {"pro-pkg"}}},
	}

	if err := inst.installMain(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("installMain: %v", err)
	}
	want := []string{"install", "-y", "base-pkg", "pro-pkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

func TestRemove_usesVariantPackages(t *testing.T) {
	var gotArgs []string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			gotArgs = append([]string{}, args...)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"base-pkg"},
		Apt:      &pkg.AptConfig{Variant: "pro", Variants: map[string][]string{"pro": {"pro-pkg"}}},
	}

	if err := inst.Remove(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := []string{"remove", "-y", "base-pkg", "pro-pkg"}
	if len(gotArgs) != len(want) {
		t.Fatalf("got %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("arg %d: got %q, want %q", i, gotArgs[i], want[i])
		}
	}
}

// ---- aptpty.AptExecFunc type check -----------------------------------------

func TestAptExecFunc_signature(t *testing.T) {
	var _ aptpty.AptExecFunc = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
}
