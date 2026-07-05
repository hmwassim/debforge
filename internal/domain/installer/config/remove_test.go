package config

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/userdir"
)

func TestRemove_configs(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	homeDir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatal(err)
	}
	expandedPath := filepath.Join(homeDir, ".config/bar.conf")
	fs.files[expandedPath] = []byte("user content")
	fs.files["/etc/foo.conf"] = []byte("content")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		ConfigHashes: map[string]string{
			expandedPath:    hashContent("user content"),
			"/etc/foo.conf": hashContent("content"),
		},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
		RemoveConfigs: map[string]string{
			"/etc/removed.conf": "",
		},
		Configs: map[string]string{
			"/etc/foo.conf": "content",
		},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err = inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	expected := []string{expandedPath, "/etc/removed.conf", "/etc/foo.conf"}
	for _, e := range expected {
		found := false
		for _, r := range removed {
			if r == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be removed, got %v", e, removed)
		}
	}
}

func TestRemove_skipModifiedUserConfig(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}

	homeDir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatal(err)
	}
	expandedPath := filepath.Join(homeDir, ".config/bar.conf")
	fs.files[expandedPath] = []byte("modified content")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		ConfigHashes: map[string]string{
			expandedPath: hashContent("original content"),
		},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "original content",
		},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err = inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals for modified user config, got %v", removed)
	}
}

func TestRemove_removeAllError(t *testing.T) {
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		return fmt.Errorf("remove failed")
	}
	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err := inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error from RemoveAll")
	}
}
