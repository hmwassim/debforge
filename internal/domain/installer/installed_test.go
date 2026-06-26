package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestCheckInstalled_aptVariantOnlyInstalled(t *testing.T) {
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variant:  "stable",
			Variants: map[string]string{"stable": "real-system-pkg"},
		},
	}

	if !CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, p) {
		t.Error("expected CheckInstalled=true for variant-resolved package that is installed")
	}
}

func TestCheckInstalled_aptVariantOnlyNotInstalled(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" && strings.Contains(strings.Join(args, " "), "real-system-pkg") {
				return []byte("not-installed\n"), nil, nil
			}
			return []byte("installed\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeApt,
		Apt: &pkg.AptConfig{
			Variant:  "stable",
			Variants: map[string]string{"stable": "real-system-pkg"},
		},
	}

	if CheckInstalled(context.Background(), runner, nil, p) {
		t.Error("expected CheckInstalled=false for variant-resolved package that is not installed")
	}
}

func TestCheckInstalled_aptVariantAndPackages(t *testing.T) {
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"base-pkg"},
		Apt: &pkg.AptConfig{
			Variant:  "stable",
			Variants: map[string]string{"stable": "variant-pkg"},
		},
	}

	if !CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, p) {
		t.Error("expected CheckInstalled=true when both Packages and variant-resolved package are installed")
	}
}

func TestCheckInstalled_aptNoVariantFallsBackToPrimary(t *testing.T) {
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"real-system-pkg"},
	}

	if !CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, p) {
		t.Error("expected CheckInstalled=true via PrimarySystemPackage when no variant is set")
	}
}

func TestCheckInstalled_aptNoVariantNoPackagesFallsBackToName(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, errors.New("not found")
			}
			return nil, nil, nil
		},
	}
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	}

	if CheckInstalled(context.Background(), runner, nil, p) {
		t.Error("expected CheckInstalled=false when no variant and no packages are set")
	}
}
