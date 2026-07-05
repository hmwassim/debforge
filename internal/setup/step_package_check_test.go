package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- I386Step tests --------------------------------------------------------

func TestI386Step_CheckSatisfied(t *testing.T) {
	step := &I386Step{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("arm64\ni386\n"), nil, nil
		},
	}
	result := step.Check(context.Background(), &Context{Runner: runner})
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestI386Step_CheckMissing(t *testing.T) {
	step := &I386Step{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("arm64\n"), nil, nil
		},
	}
	result := step.Check(context.Background(), &Context{Runner: runner})
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestI386Step_CheckError(t *testing.T) {
	step := &I386Step{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("dpkg not found")
		},
	}
	result := step.Check(context.Background(), &Context{Runner: runner})
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestI386Step_Apply(t *testing.T) {
	step := &I386Step{}
	var calls []string
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+args[0])
			return nil, nil, nil
		},
	}
	cx := &Context{
		Runner: runner,
		UI:     &testutil.MockUI{},
	}
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(calls) < 1 || calls[0] != "dpkg --add-architecture" {
		t.Errorf("expected dpkg --add-architecture, got %v", calls)
	}
}

// ---- FirmwareStep tests ----------------------------------------------------

func TestFirmwareStep_CheckSatisfied(t *testing.T) {
	step := &FirmwareStep{}
	cx := &Context{Runner: mockDpkgRunner("installed", nil), UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestFirmwareStep_CheckMissing(t *testing.T) {
	step := &FirmwareStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestFirmwareStep_CheckError(t *testing.T) {
	step := &FirmwareStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, context.Canceled
			}
			return nil, nil, nil
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

// ---- DevtoolsStep tests ----------------------------------------------------

func TestDevtoolsStep_CheckSatisfied(t *testing.T) {
	step := &DevtoolsStep{}
	cx := &Context{Runner: mockDpkgRunner("installed", nil), UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestDevtoolsStep_CheckMissing(t *testing.T) {
	step := &DevtoolsStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestDevtoolsStep_CheckError(t *testing.T) {
	step := &DevtoolsStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, context.Canceled
			}
			return nil, nil, nil
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

// ---- KernelStep tests ------------------------------------------------------

func TestKernelStep_CheckSatisfied(t *testing.T) {
	step := &KernelStep{}
	cx := &Context{Runner: mockDpkgRunner("installed", nil), UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestKernelStep_CheckMissing(t *testing.T) {
	step := &KernelStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestKernelStep_CheckError(t *testing.T) {
	step := &KernelStep{}
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, context.Canceled
			}
			return nil, nil, nil
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := step.Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

// ---- MesaStep tests --------------------------------------------------------

func TestMesaStep_CheckSatisfied(t *testing.T) {
	cx := &Context{Runner: mockDpkgRunner("installed", nil), UI: &testutil.MockUI{}}
	result := (&MesaStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestMesaStep_CheckMissing(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := (&MesaStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestMesaStep_CheckError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, context.Canceled
			}
			return nil, nil, nil
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := (&MesaStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

// ---- MultimediaStep tests --------------------------------------------------

func TestMultimediaStep_CheckSatisfied(t *testing.T) {
	cx := &Context{Runner: mockDpkgRunner("installed", nil), UI: &testutil.MockUI{}}
	result := (&MultimediaStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestMultimediaStep_CheckMissing(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("exit status 1")
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := (&MultimediaStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestMultimediaStep_CheckError(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				return nil, nil, context.Canceled
			}
			return nil, nil, nil
		},
	}
	cx := &Context{Runner: runner, UI: &testutil.MockUI{}}
	result := (&MultimediaStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

// ---- allInstalled tests ----------------------------------------------------

func TestAllInstalled_emptyList(t *testing.T) {
	ok, err := allInstalled(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !ok {
		t.Error("expected true for empty list")
	}
}

func TestAllInstalled_notInstalledLine(t *testing.T) {
	runner := &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return []byte("installed\nnot-installed\n"), nil, nil
		},
	}
	ok, err := allInstalled(context.Background(), runner, []string{"pkg1", "pkg2"})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if ok {
		t.Error("expected false for not-installed line")
	}
}
