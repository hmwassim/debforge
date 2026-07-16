package apt

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

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

func TestNewInstaller(t *testing.T) {
	runner := &testutil.MockRunner{}
	fs := testutil.NewMockFileSystem()
	ui := &testutil.MockUI{}
	sys := &testutil.MockSystem{}
	inst := NewInstaller(runner, fs, ui, sys)
	if inst.runner != runner {
		t.Error("runner not set")
	}
	if inst.fs != fs {
		t.Error("fs not set")
	}
	if inst.ui != ui {
		t.Error("ui not set")
	}
	if inst.sys != sys {
		t.Error("sys not set")
	}
	if inst.execApt == nil {
		t.Error("execApt should not be nil")
	}
	if inst.policyCache == nil {
		t.Error("policyCache should not be nil")
	}
}

func TestCandidateVersion_cacheHit(t *testing.T) {
	callCount := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			callCount++
			return policyOutput("2.0.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner, policyCache: make(map[string]string)}

	got1, err := inst.candidateVersion(context.Background(), "test-pkg")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	got2, err := inst.candidateVersion(context.Background(), "test-pkg")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got1 != "2.0.0" || got2 != "2.0.0" {
		t.Errorf("got %q, %q, want %q", got1, got2, "2.0.0")
	}
	if callCount != 1 {
		t.Errorf("expected runner called once (cache hit), got %d calls", callCount)
	}
}

func TestCandidateVersion_cacheMiss_differentPackage(t *testing.T) {
	callCount := 0
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			callCount++
			return policyOutput("1.0.0"), nil, nil
		},
	}
	inst := &Installer{runner: runner, policyCache: make(map[string]string)}

	_, _ = inst.candidateVersion(context.Background(), "pkg-a")
	_, _ = inst.candidateVersion(context.Background(), "pkg-b")
	if callCount != 2 {
		t.Errorf("expected runner called twice for different packages, got %d", callCount)
	}
}

func TestAptExecFunc_signature(t *testing.T) {
	var _ aptpty.AptExecFunc = func(_ context.Context, _ ports.CommandRunner, _ []string, _ ports.Spinner) error {
		return nil
	}
}
