package service

import (
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
)

func TestEvalSkipCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dep        pkg.Package
		exists     bool
		rerun      bool
		wantResult skipCheck
	}{
		{
			name:       "not installed and rerun",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}},
			exists:     false,
			rerun:      true,
			wantResult: skipCheckNone,
		},
		{
			name:       "not installed and not rerun",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}},
			exists:     false,
			rerun:      false,
			wantResult: skipCheckNone,
		},
		{
			name:       "skip_update and exists",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}, SkipUpdate: true},
			exists:     true,
			rerun:      false,
			wantResult: skipCheckDisk,
		},
		{
			name:       "skip_update but force overrides",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}, SkipUpdate: true, ForceInstall: true},
			exists:     true,
			rerun:      false,
			wantResult: skipCheckDisk,
		},
		{
			name:       "skip_update but not exists",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}, SkipUpdate: true},
			exists:     false,
			rerun:      true,
			wantResult: skipCheckNone,
		},
		{
			name:       "exists and not rerun",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}},
			exists:     true,
			rerun:      false,
			wantResult: skipCheckDisk,
		},
		{
			name:       "exists and rerun falls through to variant check",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}},
			exists:     true,
			rerun:      true,
			wantResult: skipCheckNone,
		},
		{
			name:       "variant __skip__",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{Variant: "__skip__"}},
			exists:     false,
			rerun:      true,
			wantResult: skipCheckVariant,
		},
		{
			name:       "variant __skip__ not rerun exists hits disk check first",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{Variant: "__skip__"}},
			exists:     true,
			rerun:      false,
			wantResult: skipCheckDisk,
		},
		{
			name:       "nil Apt config",
			dep:        pkg.Package{Name: "foo"},
			exists:     false,
			rerun:      true,
			wantResult: skipCheckNone,
		},
		{
			name:       "normal variant not skipped",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{Variant: "stable"}},
			exists:     false,
			rerun:      true,
			wantResult: skipCheckNone,
		},
		{
			name:       "skip_update exists rerun force falls to disk check via rerun path",
			dep:        pkg.Package{Name: "foo", Apt: &pkg.AptConfig{}, SkipUpdate: true, ForceInstall: true},
			exists:     true,
			rerun:      true,
			wantResult: skipCheckNone,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evalSkipCheck(&tc.dep, tc.exists, tc.rerun)
			if got != tc.wantResult {
				t.Errorf("evalSkipCheck() = %v, want %v", got, tc.wantResult)
			}
		})
	}
}
