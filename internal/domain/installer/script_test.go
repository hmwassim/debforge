package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestRunScript_surfacesStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("configure: error: missing libfoo\n"), errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./configure", "configuring")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing libfoo") {
		t.Errorf("error should contain stderr output, got: %v", err)
	}
}

func TestRunScript_noStderr(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), ": :") {
		t.Errorf("error should not have empty stderr suffix: %v", err)
	}
}

func TestRunScript_truncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", 1000)
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte(long), errors.New("exit status 1")
		},
	}

	spinner := &mockSpinner{}
	err := RunScript(context.Background(), runner, spinner, "test-pkg", "./fail", "testing")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("long stderr should be truncated ending with ..., got: %v", msg)
	}
	if len(msg) > 600 {
		t.Errorf("error message too long (%d chars), should be truncated", len(msg))
	}
}

type mockSpinner struct {
	desc string
}

func (m *mockSpinner) Done()                  {}
func (m *mockSpinner) Fail()                  {}
func (m *mockSpinner) DoneWarn()              {}
func (m *mockSpinner) DoneInfo()              {}
func (m *mockSpinner) Pause()                 {}
func (m *mockSpinner) Resume()                {}
func (m *mockSpinner) SetDesc(d string)       { m.desc = d }
var _ interface{ SetDesc(string) } = (*mockSpinner)(nil)

func TestCheckInstalled_aptVariantOnlyInstalled(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
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

	if !CheckInstalled(context.Background(), runner, nil, p) {
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
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"base-pkg"},
		Apt: &pkg.AptConfig{
			Variant:  "stable",
			Variants: map[string]string{"stable": "variant-pkg"},
		},
	}

	if !CheckInstalled(context.Background(), runner, nil, p) {
		t.Error("expected CheckInstalled=true when both Packages and variant-resolved package are installed")
	}
}

func TestCheckInstalled_aptNoVariantFallsBackToPrimary(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed\n"), nil, nil
		},
	}
	p := &pkg.Package{
		Name:     "my-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"real-system-pkg"},
	}

	if !CheckInstalled(context.Background(), runner, nil, p) {
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
