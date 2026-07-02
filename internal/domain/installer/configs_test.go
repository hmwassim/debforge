package installer

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

type mockSys struct{}

func (m *mockSys) IsPrivileged() bool           { return false }
func (m *mockSys) Getenv(_ string) string       { return "" }
func (m *mockSys) UserHomeDir() (string, error) { return "/home/test", nil }
func (m *mockSys) LookupUser(_ string) (*ports.UserInfo, error) {
	return &ports.UserInfo{HomeDir: "/home/test", Uid: 1000, Gid: 1000}, nil
}

var testSys = &mockSys{}

func TestHasHomePrefix(t *testing.T) {
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
		got := HasHomePrefix(tc.path)
		if got != tc.want {
			t.Errorf("HasHomePrefix(%q) = %v, want %v", tc.path, got, tc.want)
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
		got := ExpandHome(tc.path, tc.homeDir)
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

func TestDecideConfigAction_noBaseline(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("some content")
	got := DecideConfigAction(fs, "/etc/foo.conf", "new content", "", false)
	if got != ConfigSkip {
		t.Errorf("expected ConfigSkip when no baseline, got %v", got)
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
	return sha256Hex([]byte(content))
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
	homeDir, err := UserHomeDir(testSys)
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
	homeDir, err := UserHomeDir(testSys)
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

func TestUserHomeDir_default(t *testing.T) {
	dir, err := UserHomeDir(testSys)
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if dir != "/home/test" {
		t.Errorf("expected /home/test, got %q", dir)
	}
}
