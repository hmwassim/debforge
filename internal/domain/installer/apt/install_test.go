package apt

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

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
	p := &pkg.Package{Name: "test-pkg", Apt: &pkg.AptConfig{Backports: []string{"bpkg"}}}

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

func TestInstall_success(t *testing.T) {
	var execCalls [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-cache" && len(args) >= 2 && args[0] == "policy" {
				return policyOutput("2.0.0"), nil, nil
			}
			if name == "extrepo" {
				return nil, nil, nil
			}
			if name == "apt-get" && args[0] == "update" {
				return nil, nil, nil
			}
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{
		runner: runner,
		fs:     fs,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			execCalls = append(execCalls, append([]string{}, args...))
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(execCalls) == 0 {
		t.Fatal("expected execApt to be called")
	}
}

func TestInstall_gpuCheckError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("lspci not found")
		},
	}
	inst := &Installer{runner: runner}
	p := &pkg.Package{
		Name:     "nvidia",
		Type:     pkg.TypeApt,
		Packages: []string{"nvidia-driver"},
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err == nil {
		t.Fatal("expected error when GPU check fails")
	}
}

func TestInstall_alreadyUpToDate(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-cache" && len(args) >= 2 && args[0] == "policy" {
				return policyOutput("1.0.0"), nil, nil
			}
			return nil, nil, nil
		},
	}
	inst := &Installer{
		runner: runner,
		execApt: func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
			t.Fatal("execApt should not be called when already up to date")
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Version:  "1.0.0",
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
}

func TestInstall_versionCapture(t *testing.T) {
	var execCalls [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-cache" && len(args) >= 2 && args[0] == "policy" {
				return policyOutput("2.0.0"), nil, nil
			}
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{
		runner: runner,
		fs:     fs,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			execCalls = append(execCalls, args)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"pkg-a"},
		Apt:      &pkg.AptConfig{},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p.Version != "2.0.0" {
		t.Errorf("expected p.Version to be captured as 2.0.0, got %q", p.Version)
	}
}

func TestInstall_variantVersionCapture(t *testing.T) {
	var execCalls [][]string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "apt-cache" && len(args) >= 2 && args[0] == "policy" {
				if args[1] == "variant-pkg" {
					return policyOutput("3.0.0"), nil, nil
				}
				return policyOutput("1.0.0"), nil, nil
			}
			return nil, nil, nil
		},
	}
	fs := testutil.NewMockFileSystem()
	inst := &Installer{
		runner: runner,
		fs:     fs,
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			execCalls = append(execCalls, args)
			return nil
		},
	}
	p := &pkg.Package{
		Name:     "test-pkg",
		Type:     pkg.TypeApt,
		Packages: []string{"base-pkg"},
		Apt: &pkg.AptConfig{
			Variant:  "pro",
			Variants: map[string][]string{"pro": {"variant-pkg"}},
		},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p.Version != "3.0.0" {
		t.Errorf("expected p.Version to capture variant package version 3.0.0, got %q", p.Version)
	}
}

func TestInstall_forceInstallSkipsUpToDateCheck(t *testing.T) {
	var execCalls [][]string
	inst := &Installer{
		execApt: func(_ context.Context, _ ports.CommandRunner, args []string, _ ports.Spinner) error {
			execCalls = append(execCalls, args)
			return nil
		},
	}
	p := &pkg.Package{
		Name:         "test-pkg",
		Type:         pkg.TypeApt,
		Packages:     []string{"pkg-a"},
		Version:      "1.0.0",
		ForceInstall: true,
		Apt:          &pkg.AptConfig{},
	}

	if err := inst.Install(context.Background(), p, &testutil.MockSpinner{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(execCalls) == 0 {
		t.Error("expected execApt to be called despite being up to date")
	}
}

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
