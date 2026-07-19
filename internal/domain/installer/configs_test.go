package installer

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/textutil"
	"github.com/hmwassim/debforge/internal/userdir"
)

var testSys = &testutil.MockSystem{}

func TestUserdirHasPrefix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"~/foo", true},
		{"~", true},
		{"/home/user", false},
		{"", false},
		{"/~/foo", false},
	}
	for _, tc := range tests {
		got := userdir.HasPrefix(tc.path)
		if got != tc.want {
			t.Errorf("HasPrefix(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		path    string
		homeDir string
		want    string
	}{
		{"~/foo/bar", "/home/user", "/home/user/foo/bar"},
		{"~", "/home/user", "/home/user"},
		{"/etc/foo", "/home/user", "/etc/foo"},
		{"", "/home/user", ""},
	}
	for _, tc := range tests {
		got := userdir.ExpandHome(tc.path, tc.homeDir)
		if got != tc.want {
			t.Errorf("ExpandHome(%q, %q) = %q, want %q", tc.path, tc.homeDir, got, tc.want)
		}
	}
}

func TestDecideConfigAction_forceInstall(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	got := DecideConfigAction(fs, "/any/path", "content", "", true)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when ForceInstall is true, got %v", got)
	}
}

func TestDecideConfigAction_notExists(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	got := DecideConfigAction(fs, "/nonexistent", "content", "", false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when file does not exist, got %v", got)
	}
}

func TestDecideConfigAction_allMatch(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("content")
	baseline := hashContent("content")
	got := DecideConfigAction(fs, "/etc/foo.conf", "content", baseline, false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when all match, got %v", got)
	}
}

func TestDecideConfigAction_diskUnchangedPackageChanged(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("old")
	baseline := hashContent("old")
	got := DecideConfigAction(fs, "/etc/foo.conf", "new", baseline, false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when user didn't touch it, got %v", got)
	}
}

func TestDecideConfigAction_userModifiedPackageUnchanged(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("user edited")
	baseline := hashContent("original")
	got := DecideConfigAction(fs, "/etc/foo.conf", "original", baseline, false)
	if got != ConfigSkip {
		t.Errorf("expected ConfigSkip when user modified, got %v", got)
	}
}

func TestDecideConfigAction_bothModified(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("user edited")
	baseline := hashContent("original")
	got := DecideConfigAction(fs, "/etc/foo.conf", "new version", baseline, false)
	if got != ConfigConflict {
		t.Errorf("expected ConfigConflict when both changed, got %v", got)
	}
}

func TestDecideConfigAction_noBaselineDiffers(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("existing content")
	got := DecideConfigAction(fs, "/etc/foo.conf", "package content", "", false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when no baseline and content differs, got %v", got)
	}
}

func TestDecideConfigAction_noBaselineMatches(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("same content")
	got := DecideConfigAction(fs, "/etc/foo.conf", "same content", "", false)
	if got != ConfigSkip {
		t.Errorf("expected ConfigSkip when no baseline but content matches, got %v", got)
	}
}

func TestDecideConfigAction_existsError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) {
		return false, errors.New("stat failed")
	}
	got := DecideConfigAction(fs, "/etc/foo.conf", "content", "anything", false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when Exists errors, got %v", got)
	}
}

func TestDecideConfigAction_readError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) {
		return true, nil
	}
	// File not in Files map, so ReadFile returns error
	got := DecideConfigAction(fs, "/etc/foo.conf", "content", "baseline", false)
	if got != ConfigWrite {
		t.Errorf("expected ConfigWrite when ReadFile errors, got %v", got)
	}
}

func hashContent(content string) string {
	return textutil.Sha256Hex([]byte(content))
}

func TestWriteConfigs(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	p := &pkg.Package{
		Name: "test-pkg",
		Configs: map[string]string{
			"/etc/foo/bar.conf": "content",
		},
	}
	err := WriteConfigs(fs, &testutil.MockSpinner{}, p)
	if err != nil {
		t.Fatalf("WriteConfigs: %v", err)
	}
	if string(fs.Files["/etc/foo/bar.conf"]) != "content" {
		t.Errorf("file content = %q, want %q", string(fs.Files["/etc/foo/bar.conf"]), "content")
	}
}

func TestWriteConfigs_empty(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	p := &pkg.Package{Name: "test-pkg"}
	err := WriteConfigs(fs, &testutil.MockSpinner{}, p)
	if err != nil {
		t.Errorf("WriteConfigs empty: %v", err)
	}
}

func TestWriteConfigs_mkdirError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.MkdirAllFunc = func(_ string, _ int) error {
		return errors.New("mkdir failed")
	}
	p := &pkg.Package{
		Name: "test-pkg",
		Configs: map[string]string{
			"/etc/foo/bar.conf": "content",
		},
	}
	err := WriteConfigs(fs, &testutil.MockSpinner{}, p)
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestWriteUserConfigs(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	homeDir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatal(err)
	}
	p := &pkg.Package{
		Name: "test-pkg",
		UserConfigs: map[string]string{
			"~/.config/foo.conf": "user content",
		},
	}
	err = WriteUserConfigs(fs, testSys, &testutil.MockSpinner{}, p)
	if err != nil {
		t.Fatalf("WriteUserConfigs: %v", err)
	}
	expandedPath := filepath.Join(homeDir, ".config/foo.conf")
	if string(fs.Files[expandedPath]) != "user content" {
		t.Errorf("expected file at %s with content %q, got %q",
			expandedPath, "user content", string(fs.Files[expandedPath]))
	}
}

func TestWriteUserConfigs_empty(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	p := &pkg.Package{Name: "test-pkg"}
	err := WriteUserConfigs(fs, testSys, &testutil.MockSpinner{}, p)
	if err != nil {
		t.Errorf("WriteUserConfigs empty: %v", err)
	}
}

func TestWriteUserConfigs_existingWithBaseline(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	homeDir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatal(err)
	}
	expandedPath := filepath.Join(homeDir, ".config/foo.conf")
	fs.Files[expandedPath] = []byte("user content")

	hashes := map[string]string{expandedPath: hashContent("user content")}

	p := &pkg.Package{
		Name: "test-pkg",
		UserConfigs: map[string]string{
			"~/.config/foo.conf": "user content",
		},
	}
	updated, err := WriteUserConfigsWithHashes(fs, testSys, &testutil.MockSpinner{}, p, hashes)
	if err != nil {
		t.Fatalf("WriteUserConfigsWithHashes: %v", err)
	}
	if string(fs.Files[expandedPath]) != "user content" {
		t.Errorf("expected file content to be 'user content'")
	}
	if updated[expandedPath] != hashes[expandedPath] {
		t.Errorf("expected hash to be updated, got %q", updated[expandedPath])
	}
}

func TestFileIsModified_forceInstall(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	if FileIsModified(fs, "/any/path", "content", true) {
		t.Error("expected false when forceInstall is true")
	}
}

func TestFileIsModified_notExists(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	if FileIsModified(fs, "/nonexistent", "content", false) {
		t.Error("expected false when file does not exist")
	}
}

func TestFileIsModified_existsError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) {
		return false, errors.New("stat failed")
	}
	if FileIsModified(fs, "/any", "content", false) {
		t.Error("expected false when Exists errors")
	}
}

func TestFileIsModified_readError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) { return true, nil }
	// File not in Files map so ReadFile fails
	if FileIsModified(fs, "/some/file", "other", false) {
		t.Error("expected false when ReadFile errors")
	}
}

func TestFileIsModified_unchanged(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/some/file"] = []byte("same content")
	if FileIsModified(fs, "/some/file", "same content", false) {
		t.Error("expected false when content matches")
	}
}

func TestFileIsModified_modified(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/some/file"] = []byte("old content")
	if !FileIsModified(fs, "/some/file", "new content", false) {
		t.Error("expected true when content differs")
	}
}

func TestWriteUserConfigsWithHashes_chownDirError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	sys := &testutil.MockSystem{
		Privileged: true,
		Env:        map[string]string{"SUDO_USER": "testuser"},
	}
	fs.MkdirAllFunc = func(_ string, _ int) error { return nil }
	fs.ChownFunc = func(path string, uid, gid int) error {
		return errors.New("chown failed")
	}
	p := &pkg.Package{
		Name: "test-pkg",
		UserConfigs: map[string]string{
			"~/.config/foo.conf": "content",
		},
	}
	_, err := WriteUserConfigsWithHashes(fs, sys, &testutil.MockSpinner{}, p, nil)
	if err == nil || !strings.Contains(err.Error(), "chown") {
		t.Fatalf("expected chown error, got %v", err)
	}
}

func TestValidateConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid /etc", "/etc/foo.conf", false},
		{"valid /usr/share", "/usr/share/app.conf", false},
		{"valid /usr/lib", "/usr/lib/foo/bar.conf", false},
		{"valid /opt", "/opt/app/config.yaml", false},
		{"valid /boot", "/boot/grub.cfg", false},
		{"valid /var", "/var/log/app.conf", false},
		{"empty path", "", true},
		{"relative path", "etc/foo.conf", true},
		{"traversal in path", "/etc/../etc/foo.conf", true},
		{"traversal at end", "/etc/foo/../../etc/foo.conf", true},
		{"outside allowed /root", "/root/.ssh/authorized_keys", true},
		{"outside allowed /home", "/home/user/.bashrc", true},
		{"outside allowed /tmp", "/tmp/evil.conf", true},
		{"outside allowed /srv", "/srv/share.conf", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigPath(%q) err = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUserConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		absPath string
		homeDir string
		wantErr bool
	}{
		{"valid user config", "/home/user/.config/app.conf", "/home/user", false},
		{"valid nested", "/home/user/.config/foo/bar.conf", "/home/user", false},
		{"escapes home via traversal", "/home/user/../../etc/passwd", "/home/user", true},
		{"outside home absolute", "/etc/foo.conf", "/home/user", true},
		{"outside home other user", "/home/other/.config/app.conf", "/home/user", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserConfigPath(tt.absPath, tt.homeDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUserConfigPath(%q, %q) err = %v, wantErr %v",
					tt.absPath, tt.homeDir, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRemovablePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid file", "/opt/debforge/bin/foo", false},
		{"valid subdir", "/etc/apt/sources.list.d/foo.list", false},
		{"empty path", "", true},
		{"root", "/", true},
		{"bin", "/bin", true},
		{"boot", "/boot", true},
		{"dev", "/dev", true},
		{"etc", "/etc", true},
		{"home", "/home", true},
		{"lib", "/lib", true},
		{"lib64", "/lib64", true},
		{"opt", "/opt", true},
		{"proc", "/proc", true},
		{"root home", "/root", true},
		{"run", "/run", true},
		{"sbin", "/sbin", true},
		{"sys", "/sys", true},
		{"usr", "/usr", true},
		{"var", "/var", true},
		{"traversal", "/opt/foo/../../etc/passwd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemovablePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRemovablePath(%q) err = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestUserHomeDir_default(t *testing.T) {
	dir, err := userdir.Home(testSys)
	if err != nil {
		t.Fatalf("userdir.Home: %v", err)
	}
	if dir != "/home/test" {
		t.Errorf("expected /home/test, got %q", dir)
	}
}
