package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- mock step -------------------------------------------------------------

type mockStep struct {
	name        string
	checkResult CheckResult
	applyErr    error
	applyCalled bool
}

func (s *mockStep) Name() string                                    { return s.name }
func (s *mockStep) Check(_ context.Context, _ *Context) CheckResult { return s.checkResult }
func (s *mockStep) Apply(_ context.Context, _ *Context, _ CheckResult) error {
	s.applyCalled = true
	return s.applyErr
}

// ---- Runner tests ----------------------------------------------------------

func TestRunner_Satisfied(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if step.applyCalled {
		t.Error("Apply should not be called for satisfied step")
	}
}

func TestRunner_Missing(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusMissing, Summary: "not found"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for missing step")
	}
}

func TestRunner_Drifted(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusDrifted, Summary: "modified"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for drifted step")
	}
}

func TestRunner_Conflict(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusConflict, Summary: "conflict"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called for conflict step")
	}
}

func TestRunner_Error(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusError, Summary: "check failed"}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if step.applyCalled {
		t.Error("Apply should not be called for error step")
	}
}

func TestRunner_ApplyError(t *testing.T) {
	step := &mockStep{
		name:        "test",
		checkResult: CheckResult{Status: StatusMissing, Summary: "not found"},
		applyErr:    errors.New("apply failed"),
	}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunner_ForceSkipsCheck(t *testing.T) {
	step := &mockStep{name: "test", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step)
	err := runner.Run(context.Background(), &Context{
		UI:    &testutil.MockUI{},
		Force: true,
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !step.applyCalled {
		t.Error("Apply should be called in force mode even for satisfied step")
	}
}

func TestRunner_StopsOnError(t *testing.T) {
	step1 := &mockStep{name: "first", checkResult: CheckResult{Status: StatusSatisfied}}
	step2 := &mockStep{name: "second", checkResult: CheckResult{Status: StatusError, Summary: "boom"}}
	step3 := &mockStep{name: "third", checkResult: CheckResult{Status: StatusSatisfied}}
	runner := NewRunner(step1, step2, step3)
	err := runner.Run(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if step3.applyCalled {
		t.Error("runner should stop after step2 error")
	}
}

func TestCheckAll(t *testing.T) {
	step1 := &mockStep{name: "a", checkResult: CheckResult{Status: StatusSatisfied}}
	step2 := &mockStep{name: "b", checkResult: CheckResult{Status: StatusMissing}}
	runner := NewRunner(step1, step2)
	results := runner.CheckAll(context.Background(), &Context{
		UI: &testutil.MockUI{},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", results[0].Status)
	}
	if results[1].Status != StatusMissing {
		t.Errorf("expected missing, got %v", results[1].Status)
	}
}

// ---- State tests -----------------------------------------------------------

func TestLoadState_NotFound(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	st, err := LoadState(fs, "/nonexistent/setup_state.json")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if st.ConfigHashes == nil {
		t.Error("ConfigHashes should be initialized")
	}
}

func TestLoadState_Existing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	path := "/var/setup_state.json"
	fs.Files[path] = []byte(`{"config_hashes":{"/etc/foo.conf":"abc123"}}`)
	st, err := LoadState(fs, path)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if st.ConfigHashes["/etc/foo.conf"] != "abc123" {
		t.Errorf("expected abc123, got %q", st.ConfigHashes["/etc/foo.conf"])
	}
}

func TestSaveAndLoadState(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	path := "/var/setup_state.json"
	st := &State{ConfigHashes: map[string]string{"/a": "hash1", "/b": "hash2"}}
	if err := SaveState(fs, path, st); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadState(fs, path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ConfigHashes["/a"] != "hash1" {
		t.Errorf("expected hash1, got %q", loaded.ConfigHashes["/a"])
	}
	if loaded.ConfigHashes["/b"] != "hash2" {
		t.Errorf("expected hash2, got %q", loaded.ConfigHashes["/b"])
	}
}

// ---- ReposStep tests -------------------------------------------------------

func newReposCx(fs *testutil.MockFileSystem, force bool) *Context {
	if fs == nil {
		fs = testutil.NewMockFileSystem()
	}
	return &Context{
		Fsys:         fs,
		Runner:       &testutil.MockRunner{RunFunc: func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) { return nil, nil, nil }},
		UI:           &testutil.MockUI{},
		Force:        force,
		ConfigHashes: make(map[string]string),
	}
}

func TestReposStep_CheckMissing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: "deb ..."}}}
	result := step.Check(context.Background(), newReposCx(fs, false))
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestReposStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := "deb ..."
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(content)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: content}}}
	cx := newReposCx(fs, false)
	// Record the hash so DecideConfigAction sees it as matching
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = installer.Sha256Hex([]byte(content))
	result := step.Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestReposStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	original := "deb http://old.example.com"
	modified := "deb http://modified.example.com"
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(modified)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: original}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = installer.Sha256Hex([]byte(original))
	result := step.Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestReposStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "deb http://modified.example.com"
	newContent := "deb http://new.example.com"
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(modified)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: newContent}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = installer.Sha256Hex([]byte("deb http://original.example.com"))
	result := step.Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestReposStep_ApplyMissing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := "deb http://deb.debian.org/debian trixie main"
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: content}}}
	cx := newReposCx(fs, false)
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := fs.ReadFile("/etc/apt/sources.list.d/debian.sources")
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
	if cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] == "" {
		t.Error("hash should be recorded")
	}
}

func TestReposStep_ApplyForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	original := "deb http://old.example.com"
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(original)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: "deb http://new.example.com"}}}
	cx := newReposCx(fs, true)
	// With force, the hash before doesn't matter — DecideConfigAction always writes
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/apt/sources.list.d/debian.sources")
	if string(data) != "deb http://new.example.com" {
		t.Errorf("expected new content, got %q", string(data))
	}
}

func TestReposStep_ApplyConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "deb http://modified.example.com"
	newContent := "deb http://newdeb.example.com"
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(modified)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: newContent}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = installer.Sha256Hex([]byte("deb http://original.example.com"))
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Original file should be untouched
	data, _ := fs.ReadFile("/etc/apt/sources.list.d/debian.sources")
	if string(data) != modified {
		t.Errorf("original should be untouched, got %q", string(data))
	}
	// Sidecar should exist
	sidecar, err := fs.ReadFile("/etc/apt/sources.list.d/debian.sources.debforge-new")
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	if string(sidecar) != newContent {
		t.Errorf("expected sidecar content %q, got %q", newContent, string(sidecar))
	}
}

func TestReposStep_ApplyDriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "deb http://modified.example.com"
	original := "deb http://original.example.com"
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(modified)
	step := &ReposStep{Sources: []RepoSource{{Path: "/etc/apt/sources.list.d/debian.sources", Content: original}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = installer.Sha256Hex([]byte(original))
	// Drifted result means Apply is called but should not overwrite
	if err := step.Apply(context.Background(), cx, CheckResult{Status: StatusDrifted, Summary: "modified by user"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/apt/sources.list.d/debian.sources")
	if string(data) != modified {
		t.Errorf("user-modified file should not be overwritten, got %q", string(data))
	}
	// Hash should be recorded (disk hash since baseline was empty)
	if cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] == "" {
		t.Error("hash should be recorded on skip")
	}
}

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

// ---- package step helpers --------------------------------------------------

func mockDpkgRunner(result string, err error) *testutil.MockRunner {
	return &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if err != nil {
				return nil, nil, err
			}
			n := len(args) - 2
			lines := make([]byte, 0, n*10)
			for i := 0; i < n; i++ {
				lines = append(lines, result+"\n"...)
			}
			return lines, nil, nil
		},
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

// ---- config-heavy step helpers --------------------------------------------

func newPkgCfgCx(fs *testutil.MockFileSystem, runner *testutil.MockRunner) *Context {
	if fs == nil {
		fs = testutil.NewMockFileSystem()
	}
	return &Context{
		Fsys:         fs,
		Runner:       runner,
		UI:           &testutil.MockUI{},
		ConfigHashes: make(map[string]string),
	}
}

func pkgCfgRunner(pkgResult string, pkgErr error, extra func(name string, args ...string) ([]byte, []byte, error)) *testutil.MockRunner {
	return &testutil.MockRunner{
		RunFunc: func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			if name == "dpkg-query" {
				if pkgErr != nil {
					return nil, nil, pkgErr
				}
				n := len(args) - 2
				lines := make([]byte, 0, n*10)
				for i := 0; i < n; i++ {
					lines = append(lines, pkgResult+"\n"...)
				}
				return lines, nil, nil
			}
			if extra != nil {
				return extra(name, args...)
			}
			return nil, nil, nil
		},
	}
}

// ---- ExtrepoStep tests ----------------------------------------------------

func TestExtrepoStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte(extrepoConfigFiles[0].Content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = installer.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckMissing_Config(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("installed", nil, nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestExtrepoStep_CheckError(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", context.Canceled, nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = installer.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = installer.Sha256Hex([]byte("original baseline"))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestExtrepoStep_Apply_WritesConfig(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := fs.ReadFile("/etc/extrepo/config.yaml")
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if string(data) != extrepoConfigFiles[0].Content {
		t.Errorf("expected config content, got %q", string(data))
	}
	if cx.ConfigHashes["/etc/extrepo/config.yaml"] == "" {
		t.Error("hash should be recorded")
	}
}

func TestExtrepoStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = installer.Sha256Hex([]byte("original baseline"))
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/extrepo/config.yaml.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestExtrepoStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/extrepo/config.yaml"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = installer.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/extrepo/config.yaml")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
}

func TestExtrepoStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/extrepo/config.yaml")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}

// ---- ZramStep tests -------------------------------------------------------

func TestZramStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestZramStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(`[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = installer.Sha256Hex([]byte(`[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestZramStep_CheckError(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", context.Canceled, nil))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestZramStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(modified)
	original := `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = installer.Sha256Hex([]byte(original))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestZramStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = installer.Sha256Hex([]byte("original baseline"))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestZramStep_Apply_WritesConfigAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/zram-generator.conf"); err != nil {
		t.Errorf("config file not written: %v", err)
	}
	if cx.ConfigHashes["/etc/systemd/zram-generator.conf"] == "" {
		t.Error("hash should be recorded")
	}
	var foundDaemon, foundStart bool
	for _, c := range cmds {
		if c == "systemctl daemon-reload" {
			foundDaemon = true
		}
		if c == "systemctl start systemd-zram-setup@zram0.service" {
			foundStart = true
		}
	}
	if !foundDaemon {
		t.Error("expected systemctl daemon-reload")
	}
	if !foundStart {
		t.Error("expected systemctl start systemd-zram-setup@zram0.service")
	}
}

func TestZramStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	userContent := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(userContent)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = installer.Sha256Hex([]byte("original baseline"))
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) != userContent {
		t.Error("original should be untouched")
	}
	if _, err := fs.ReadFile("/etc/systemd/zram-generator.conf.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestZramStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(modified)
	original := `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = installer.Sha256Hex([]byte(original))
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
	if cx.ConfigHashes["/etc/systemd/zram-generator.conf"] == "" {
		t.Error("hash should be recorded on skip")
	}
}

func TestZramStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}

// ---- ResolvedStep tests ---------------------------------------------------

func TestResolvedStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestResolvedStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no configs), got %v", result.Status)
	}
}

func TestResolvedStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	dotContent := `[Resolve]
DNS=1.1.1.2#security.cloudflare-dns.com 1.0.0.2#security.cloudflare-dns.com 2606:4700:4700::1112#security.cloudflare-dns.com 2606:4700:4700::1002#security.cloudflare-dns.com
FallbackDNS=9.9.9.9#dns.quad9.net 149.112.112.112#dns.quad9.net 2620:fe::fe#dns.quad9.net
DNSOverTLS=yes
DNSSEC=yes
DNSStubListener=yes
MulticastDNS=no
Cache=yes
Domains=~.
`
	nmContent := `[main]
dns=systemd-resolved
`
	fs.Files["/etc/systemd/resolved.conf.d/99-dot.conf"] = []byte(dotContent)
	fs.Files["/etc/NetworkManager/conf.d/10-dns.conf"] = []byte(nmContent)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/resolved.conf.d/99-dot.conf"] = installer.Sha256Hex([]byte(dotContent))
	cx.ConfigHashes["/etc/NetworkManager/conf.d/10-dns.conf"] = installer.Sha256Hex([]byte(nmContent))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestResolvedStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/resolved.conf.d/99-dot.conf"] = []byte("user changed")
	fs.Files["/etc/NetworkManager/conf.d/10-dns.conf"] = []byte(`[main]
dns=systemd-resolved
`)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/resolved.conf.d/99-dot.conf"] = installer.Sha256Hex([]byte("user changed"))
	cx.ConfigHashes["/etc/NetworkManager/conf.d/10-dns.conf"] = installer.Sha256Hex([]byte("original"))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestResolvedStep_Apply_WritesConfigsAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		cmds = append(cmds, cmd)
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ResolvedStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/resolved.conf.d/99-dot.conf"); err != nil {
		t.Errorf("99-dot.conf not written: %v", err)
	}
	if _, err := fs.ReadFile("/etc/NetworkManager/conf.d/10-dns.conf"); err != nil {
		t.Errorf("10-dns.conf not written: %v", err)
	}
	expected := []string{
		"ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf",
		"systemctl enable --now systemd-resolved",
		"nmcli general reload",
		"systemctl restart systemd-resolved",
		"resolvectl query debian.org",
	}
	for _, e := range expected {
		found := false
		for _, c := range cmds {
			if c == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %q not found in %v", e, cmds)
		}
	}
}

// ---- TimesyncdStep tests --------------------------------------------------

func TestTimesyncdStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := `[Time]
NTP=0.debian.pool.ntp.org 1.debian.pool.ntp.org
FallbackNTP=2.debian.pool.ntp.org 3.debian.pool.ntp.org
`
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte(content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = installer.Sha256Hex([]byte(content))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte(modified)
	original := `[Time]
NTP=time.cloudflare.com
FallbackNTP=time.google.com 0.debian.pool.ntp.org 1.debian.pool.ntp.org 2.debian.pool.ntp.org 3.debian.pool.ntp.org
`
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = installer.Sha256Hex([]byte(original))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = installer.Sha256Hex([]byte("original baseline"))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestTimesyncdStep_Apply_WritesConfigAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		cmds = append(cmds, cmd)
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&TimesyncdStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"); err != nil {
		t.Errorf("config not written: %v", err)
	}
	var foundEnable, foundTimedate bool
	for _, c := range cmds {
		if c == "systemctl enable --now systemd-timesyncd" {
			foundEnable = true
		}
		if c == "timedatectl set-ntp true" {
			foundTimedate = true
		}
	}
	if !foundEnable {
		t.Error("expected systemctl enable --now systemd-timesyncd")
	}
	if !foundTimedate {
		t.Error("expected timedatectl set-ntp true")
	}
}

// ---- DesktopStep helpers & tests ------------------------------------------

type mockSysDesktop struct {
	env map[string]string
}

func (m *mockSysDesktop) IsPrivileged() bool                          { return false }
func (m *mockSysDesktop) Getenv(key string) string                    { return m.env[key] }
func (m *mockSysDesktop) UserHomeDir() (string, error)                { return "/home/user", nil }
func (m *mockSysDesktop) LookupUser(name string) (*ports.UserInfo, error) { return nil, nil }

func desktopCx(runner *testutil.MockRunner, de string) *Context {
	fs := testutil.NewMockFileSystem()
	return desktopCxWithFs(fs, runner, de)
}

func desktopCxWithFs(fs *testutil.MockFileSystem, runner *testutil.MockRunner, de string) *Context {
	return &Context{
		Fsys:         fs,
		Runner:       runner,
		Sys:          &mockSysDesktop{env: map[string]string{"XDG_CURRENT_DESKTOP": de}},
		UI:           &testutil.MockUI{},
		ConfigHashes: make(map[string]string),
	}
}

func desktopCxSatisfied(runner *testutil.MockRunner, de string) *Context {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{}        // dir marker
	fs.Files["/home/user/.bashrc"] = bashrcDBlock
	return desktopCxWithFs(fs, runner, de)
}

func TestDesktopStep_CheckSatisfied_KDE(t *testing.T) {
	cx := desktopCxSatisfied(pkgCfgRunner("installed", nil, nil), "KDE")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestDesktopStep_CheckSatisfied_GNOME(t *testing.T) {
	cx := desktopCxSatisfied(pkgCfgRunner("installed", nil, nil), "GNOME")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestDesktopStep_CheckSatisfied_UnknownDE(t *testing.T) {
	cx := desktopCxSatisfied(pkgCfgRunner("installed", nil, nil), "")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestDesktopStep_CheckMissing_Packages(t *testing.T) {
	cx := desktopCx(pkgCfgRunner("", fmt.Errorf("exit 1"), nil), "KDE")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestDesktopStep_CheckMissing_BashrcDDir(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	// No bashrc.d dir set up
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "KDE")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing for missing bashrc.d dir, got %v", result.Status)
	}
}

func TestDesktopStep_CheckMissing_BashrcBlock(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{} // dir marker
	fs.Files["/home/user/.bashrc"] = []byte("existing content without the block")
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "KDE")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing for missing block, got %v", result.Status)
	}
}

func TestDesktopStep_CheckError(t *testing.T) {
	cx := desktopCx(pkgCfgRunner("", context.Canceled, nil), "KDE")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestDesktopStep_Apply_CreatesBashrcD(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "")
	if err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := fs.ReadFile("/home/user/.bashrc")
	if err != nil {
		t.Fatalf(".bashrc not written: %v", err)
	}
	if !bytes.Contains(data, bashrcDBlock) {
		t.Error("bashrc.d loader block not found in .bashrc")
	}
}

func TestDesktopStep_Apply_ReplacesBlock(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	oldBlock := []byte(bashrcDStartMarker + `if [ -d "$HOME/.config/bashrc.d" ]; then
    for file in "$HOME/.config/bashrc.d"/*.sh; do
        [ -f "$file" ] && . "$file"
    done
fi` + bashrcDEndMarker)
	fs.Files["/home/user/.config/bashrc.d"] = []byte{}
	fs.Files["/home/user/.bashrc"] = []byte("header\n" + string(oldBlock) + "\nfooter")
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "")
	if err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/home/user/.bashrc")
	if !bytes.HasPrefix(data, []byte("header\n")) {
		t.Error("content before block should be preserved")
	}
	if !bytes.HasSuffix(data, []byte("\nfooter")) {
		t.Error("content after block should be preserved")
	}
	if !bytes.Contains(data, bashrcDBlock) {
		t.Error("bashrc.d loader block not found after replace")
	}
}

func TestDesktopStep_Apply_AppendsBlock(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{}
	fs.Files["/home/user/.bashrc"] = []byte("existing content")
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "")
	if err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/home/user/.bashrc")
	if !strings.Contains(string(data), "existing content") {
		t.Error("existing content should be preserved")
	}
	if !bytes.Contains(data, bashrcDBlock) {
		t.Error("bashrc.d loader block not found after append")
	}
}

func TestDesktopStep_Apply_Idempotent(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "")
	if err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	data, _ := fs.ReadFile("/home/user/.bashrc")
	count := strings.Count(string(data), bashrcDStartMarker)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of start marker, got %d", count)
	}
}

// ---- MesaStep tests -------------------------------------------------------

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

// ---- MultimediaStep tests -------------------------------------------------

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

// ---- FontsStep tests ------------------------------------------------------

func TestFontsStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestFontsStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte(fontsConfigFiles[0].Content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = installer.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestFontsStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestFontsStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = installer.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestFontsStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = installer.Sha256Hex([]byte("original baseline"))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestFontsStep_Apply_WritesConfigAndRunsFcCache(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/fonts/local.conf"); err != nil {
		t.Errorf("config file not written: %v", err)
	}
	if cx.ConfigHashes["/etc/fonts/local.conf"] == "" {
		t.Error("hash should be recorded")
	}
	found := false
	for _, c := range cmds {
		if c == "fc-cache -f" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected fc-cache -f")
	}
}

func TestFontsStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = installer.Sha256Hex([]byte("original baseline"))
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/fonts/local.conf.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestFontsStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/fonts/local.conf"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = installer.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/fonts/local.conf")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
}

func TestFontsStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/fonts/local.conf")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}
