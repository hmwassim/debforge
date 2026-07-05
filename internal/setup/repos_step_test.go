package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/textutil"
)

// ---- ReposStep tests -------------------------------------------------------

func TestReposStep_CheckMissing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: "deb ..."}}}
	result := step.Check(context.Background(), newReposCx(fs, false))
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestReposStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := "deb ..."
	fs.Files["/etc/apt/sources.list.d/debian.sources"] = []byte(content)
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: content}}}
	cx := newReposCx(fs, false)
	// Record the hash so DecideConfigAction sees it as matching
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = textutil.Sha256Hex([]byte(content))
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
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: original}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = textutil.Sha256Hex([]byte(original))
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
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: newContent}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = textutil.Sha256Hex([]byte("deb http://original.example.com"))
	result := step.Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

func TestReposStep_ApplyMissing(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := "deb http://deb.debian.org/debian trixie main"
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: content}}}
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
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: "deb http://new.example.com"}}}
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
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: newContent}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = textutil.Sha256Hex([]byte("deb http://original.example.com"))
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
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list.d/debian.sources", Content: original}}}
	cx := newReposCx(fs, false)
	cx.ConfigHashes["/etc/apt/sources.list.d/debian.sources"] = textutil.Sha256Hex([]byte(original))
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

// ---- ReposStep Apply error paths ------------------------------------------

func TestReposStep_ApplyWriteError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.WriteFileFunc = func(_ string, _ []byte, _ int) error {
		return errors.New("write denied")
	}
	step := &ReposStep{Sources: []ConfigFile{{Path: "/etc/apt/sources.list", Content: "deb ..."}}}
	cx := newReposCx(fs, false)
	err := step.Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
