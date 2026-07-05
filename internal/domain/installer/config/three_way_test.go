package config

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestInstall_unmodifiedFilePackageUpdates(t *testing.T) {
	fs := newMockFS()
	fs.files["/etc/foo.conf"] = []byte("old content")

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: "oldhash",
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("old content"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "new content",
		},
	}

	err := (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "new content" {
		t.Errorf("expected file updated to 'new content', got %q", string(fs.files["/etc/foo.conf"]))
	}
	if p.ConfigHashes["/etc/foo.conf"] != hashContent("new content") {
		t.Errorf("expected hash updated to new content, got %q", p.ConfigHashes["/etc/foo.conf"])
	}
	if _, ok := fs.files["/etc/foo.conf.debforge-new"]; ok {
		t.Error("unexpected sidecar file")
	}
}

func TestInstall_userModifiedPackageUnchanged(t *testing.T) {
	fs := newMockFS()
	fs.files["/etc/foo.conf"] = []byte("user edited")

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: "oldhash",
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("original"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "original",
		},
	}

	err := (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "user edited" {
		t.Errorf("expected file untouched, got %q", string(fs.files["/etc/foo.conf"]))
	}
	if p.ConfigHashes["/etc/foo.conf"] != hashContent("original") {
		t.Errorf("expected hash unchanged, got %q", p.ConfigHashes["/etc/foo.conf"])
	}
	if _, ok := fs.files["/etc/foo.conf.debforge-new"]; ok {
		t.Error("unexpected sidecar file")
	}
}

func TestInstall_bothModifiedWritesSidecar(t *testing.T) {
	fs := newMockFS()
	fs.files["/etc/foo.conf"] = []byte("user edited")

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: "oldhash",
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("original"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "new version",
		},
	}

	err := (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "user edited" {
		t.Errorf("expected original untouched, got %q", string(fs.files["/etc/foo.conf"]))
	}
	sidecar, ok := fs.files["/etc/foo.conf.debforge-new"]
	if !ok {
		t.Fatal("expected sidecar file")
	}
	if string(sidecar) != "new version" {
		t.Errorf("expected sidecar content 'new version', got %q", string(sidecar))
	}
	if p.ConfigHashes["/etc/foo.conf"] != hashContent("original") {
		t.Errorf("expected hash unchanged on conflict, got %q", p.ConfigHashes["/etc/foo.conf"])
	}
}

func TestInstall_noBaselineOverwrites(t *testing.T) {
	fs := newMockFS()
	fs.files["/etc/foo.conf"] = []byte("existing disk content")

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: "oldhash",
		Configs: map[string]string{
			"/etc/foo.conf": "package content",
		},
	}

	err := (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "package content" {
		t.Errorf("expected file overwritten with package content, got %q", string(fs.files["/etc/foo.conf"]))
	}
	if p.ConfigHashes["/etc/foo.conf"] != hashContent("package content") {
		t.Errorf("expected hash of package content, got %v", p.ConfigHashes)
	}

	p.Configs["/etc/foo.conf"] = "package content"
	p.ConfigHashes["/etc/foo.conf"] = hashContent("package content")
	p.Version = "newhash"
	fs.files["/etc/foo.conf"] = []byte("package content")
	err = (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install second run: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "package content" {
		t.Errorf("expected file unchanged, got %q", string(fs.files["/etc/foo.conf"]))
	}

	p.Configs["/etc/foo.conf"] = "new package content"
	err = (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install third run: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "new package content" {
		t.Errorf("expected file updated on third run, got %q", string(fs.files["/etc/foo.conf"]))
	}
	if p.ConfigHashes["/etc/foo.conf"] != hashContent("new package content") {
		t.Errorf("expected hash updated, got %q", p.ConfigHashes["/etc/foo.conf"])
	}
}

func TestInstall_forceBypassesThreeWay(t *testing.T) {
	fs := newMockFS()
	fs.files["/etc/foo.conf"] = []byte("user edited")

	p := &pkg.Package{
		Name:         "test-config",
		Type:         pkg.TypeConfig,
		ForceInstall: true,
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("original"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "new version",
		},
	}

	err := (&Installer{fs: fs, sys: testSys}).Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if string(fs.files["/etc/foo.conf"]) != "new version" {
		t.Errorf("expected file overwritten with ForceInstall, got %q", string(fs.files["/etc/foo.conf"]))
	}
}

func TestRemove_unmodifiedFileProceeds(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	fs.files["/etc/foo.conf"] = []byte("content")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("content"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "content",
		},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err := inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 1 || removed[0] != "/etc/foo.conf" {
		t.Errorf("expected config removed, got %v", removed)
	}
}

func TestRemove_skipModifiedConfig(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	fs.files["/etc/foo.conf"] = []byte("user edited")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("original"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "original",
		},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err := inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals for modified config, got %v", removed)
	}
}

func TestRemove_configConflictSkippedNoSidecar(t *testing.T) {
	var removed []string
	fs := newMockFS()
	fs.RemoveAllFunc = func(path string) error {
		removed = append(removed, path)
		return nil
	}
	fs.files["/etc/foo.conf"] = []byte("user edited")

	p := &pkg.Package{
		Name: "test-config",
		Type: pkg.TypeConfig,
		ConfigHashes: map[string]string{
			"/etc/foo.conf": hashContent("original"),
		},
		Configs: map[string]string{
			"/etc/foo.conf": "new version",
		},
	}
	inst := &Installer{fs: fs, sys: testSys}
	err := inst.Remove(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals on conflict, got %v", removed)
	}
	if _, ok := fs.files["/etc/foo.conf.debforge-new"]; ok {
		t.Error("unexpected sidecar during removal")
	}
}
