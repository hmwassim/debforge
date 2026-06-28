package apt

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
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
