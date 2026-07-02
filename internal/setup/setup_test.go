package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
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
