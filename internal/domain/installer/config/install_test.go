package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/userdir"
)

func TestInstall_skipsWhenHashMatches(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written on hash match, got %d files", len(fs.files))
	}
	if p.Version != hash {
		t.Errorf("expected version unchanged on hash match, got %q", p.Version)
	}
}

func TestInstall_writesConfigsOnFirstInstall(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 1 {
		t.Fatalf("expected 1 file written, got %d", len(fs.files))
	}
	if string(fs.files["/etc/foo.conf"]) != "content" {
		t.Errorf("expected file content %q, got %q", "content", string(fs.files["/etc/foo.conf"]))
	}
	if p.Version == "" {
		t.Error("expected version to be set after install")
	}
}

func TestInstall_updatesVersionOnConfigChange(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}

	oldHash := computeConfigHash(&pkg.Package{
		Configs: map[string]string{"/etc/foo.conf": "old"},
	})

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Version: oldHash,
		Configs: map[string]string{"/etc/foo.conf": "old"},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written when content unchanged, got %d files", len(fs.files))
	}
	if p.Version != oldHash {
		t.Errorf("expected version unchanged, got %q", p.Version)
	}

	p.Configs["/etc/foo.conf"] = "new content"
	err = inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install after config change: %v", err)
	}

	if string(fs.files["/etc/foo.conf"]) != "new content" {
		t.Errorf("expected updated file content %q, got %q", "new content", string(fs.files["/etc/foo.conf"]))
	}
	newHash := p.Version
	if newHash == oldHash {
		t.Error("expected version to change after config change")
	}
}

func TestInstall_forceBypassesHashCheck(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}

	p := &pkg.Package{
		Name:         "test-config",
		Type:         pkg.TypeConfig,
		ForceInstall: true,
		Configs:      map[string]string{"/etc/foo.conf": "content"},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 1 {
		t.Errorf("expected 1 file written with ForceInstall, got %d files", len(fs.files))
	}
	if p.Version != hash {
		t.Errorf("expected version unchanged after force install, got %q", p.Version)
	}
}

func TestInstall_includesUserConfigsInHash(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}

	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "content"},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
	}

	hash := computeConfigHash(p)
	p.Version = hash

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(fs.files) != 0 {
		t.Errorf("expected no files written on hash match with user configs, got %d files", len(fs.files))
	}
}

func TestInstall_wrongType(t *testing.T) {
	inst := &Installer{fs: newMockFS(), sys: testSys}
	p := &pkg.Package{Name: "test", Type: pkg.TypeApt}
	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err == nil {
		t.Fatal("expected error for non-config type")
	}
}

func TestInstall_withUserConfigs(t *testing.T) {
	fs := newMockFS()
	inst := &Installer{fs: fs, sys: testSys}
	p := &pkg.Package{
		Name:    "test-config",
		Type:    pkg.TypeConfig,
		Configs: map[string]string{"/etc/foo.conf": "system content"},
		UserConfigs: map[string]string{
			"~/.config/bar.conf": "user content",
		},
	}

	err := inst.Install(context.Background(), p, &testutil.MockSpinner{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if string(fs.files["/etc/foo.conf"]) != "system content" {
		t.Errorf("system config not written correctly, got %q", string(fs.files["/etc/foo.conf"]))
	}

	homeDir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatal(err)
	}
	expandedPath := filepath.Join(homeDir, ".config/bar.conf")
	if string(fs.files[expandedPath]) != "user content" {
		t.Errorf("user config not written at %s, got %q", expandedPath, string(fs.files[expandedPath]))
	}
}
