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
			Variants: map[string][]string{"stable": {"real-system-pkg"}},
		},
	}

	ok, err := CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
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
			Variants: map[string][]string{"stable": {"real-system-pkg"}},
		},
	}

	ok, err := CheckInstalled(context.Background(), runner, nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
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
			Variants: map[string][]string{"stable": {"variant-pkg"}},
		},
	}

	ok, err := CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true when both Packages and variant-resolved package are installed")
	}
}

func TestCheckInstalled_aptNoVariantFallsBackToPrimary(t *testing.T) {
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"real-system-pkg"},
	}

	ok, err := CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
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

	ok, err := CheckInstalled(context.Background(), runner, nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected CheckInstalled=false when no variant and no packages are set")
	}
}

func TestCheckInstalled_deb_primaryInstalled(t *testing.T) {
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeDeb,
		Packages: []string{"real-system-pkg"},
	}

	ok, err := CheckInstalled(context.Background(), testutil.RunnerReturning([]byte("installed\n"), nil), nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true for deb package whose primary system package is installed")
	}
}

func TestCheckInstalled_deb_primaryNotInstalledFallsBackToName(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if strings.Contains(strings.Join(args, " "), "real-system-pkg") {
				return []byte("not-installed\n"), nil, nil
			}
			return []byte("installed\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeDeb,
		Packages: []string{"real-system-pkg"},
	}

	ok, err := CheckInstalled(context.Background(), runner, nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true when deb falls back to package name")
	}
}

func TestCheckInstalled_deb_neitherInstalled(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("not-installed\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeDeb,
		Packages: []string{"real-system-pkg"},
	}

	ok, err := CheckInstalled(context.Background(), runner, nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected CheckInstalled=false when neither primary nor name is installed")
	}
}

func TestCheckInstalled_deb_errorNonContext(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("dpkg-query failed")
		},
	}
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeDeb,
	}

	// Non-context errors from dpkg.IsInstalled are treated as
	// "not installed" (conservative), so CheckInstalled returns
	// (false, nil) rather than propagating the error.
	ok, err := CheckInstalled(context.Background(), runner, nil, nil, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected CheckInstalled=false when dpkg-query fails with non-context error")
	}
}

func TestCheckInstalled_deb_contextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := testutil.RunnerReturning(nil, context.Canceled)
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeDeb,
	}

	_, err := CheckInstalled(ctx, runner, nil, testSys, p)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestCheckInstalled_config_allFilesExist(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/my-app/config.yaml"] = []byte("key: value")
	fs.Files["/etc/my-app/defaults.conf"] = []byte("setting=true")

	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeConfig,
		Configs: map[string]string{
			"/etc/my-app/config.yaml":   "configs/my-app/config.yaml",
			"/etc/my-app/defaults.conf": "configs/my-app/defaults.conf",
		},
	}

	ok, err := CheckInstalled(context.Background(), nil, fs, testSys, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true when all config files exist")
	}
}

func TestCheckInstalled_config_oneMissing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/my-app/config.yaml"] = []byte("key: value")
	// defaults.conf is missing

	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeConfig,
		Configs: map[string]string{
			"/etc/my-app/config.yaml":   "configs/my-app/config.yaml",
			"/etc/my-app/defaults.conf": "configs/my-app/defaults.conf",
		},
	}

	ok, err := CheckInstalled(context.Background(), nil, fs, testSys, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected CheckInstalled=false when a config file is missing")
	}
}

func TestCheckInstalled_config_userConfigs(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(path string) (bool, error) {
		return true, nil
	}

	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeConfig,
		UserConfigs: map[string]string{
			"~/.config/my-app/settings.yaml": "configs/my-app/settings.yaml",
		},
	}

	ok, err := CheckInstalled(context.Background(), nil, fs, testSys, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true when user config files exist")
	}
}

func TestCheckInstalled_source(t *testing.T) {
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeSource,
	}

	ok, err := CheckInstalled(context.Background(), nil, nil, testSys, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected CheckInstalled=true for source-type package (always installed)")
	}
}

func TestCheckInstalled_aptErrorCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := testutil.RunnerReturning(nil, context.Canceled)
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeApt,
		Apt:  &pkg.AptConfig{},
	}

	_, err := CheckInstalled(ctx, runner, nil, testSys, p)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestCheckInstalled_debErrorCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := testutil.RunnerReturning(nil, context.Canceled)
	p := &pkg.Package{
		Name: "my-pkg",
		Type: pkg.TypeDeb,
	}

	_, err := CheckInstalled(ctx, runner, nil, testSys, p)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
