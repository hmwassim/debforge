package apt

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
