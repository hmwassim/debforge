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

func (m *mockSys) IsPrivileged() bool                     { return false }
func (m *mockSys) Getenv(_ string) string                  { return "" }
func (m *mockSys) UserHomeDir() (string, error)            { return "/home/test", nil }
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

func TestFileIsModified_forceInstall(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	got := FileIsModified(fs, "/any/path", "content", true)
	if got {
		t.Error("expected false when ForceInstall is true")
	}
}

func TestFileIsModified_notExists(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	got := FileIsModified(fs, "/nonexistent", "content", false)
	if got {
		t.Error("expected false when file does not exist")
	}
}

func TestFileIsModified_matches(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("content")
	got := FileIsModified(fs, "/etc/foo.conf", "content", false)
	if got {
		t.Error("expected false when content matches")
	}
}

func TestFileIsModified_differs(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/foo.conf"] = []byte("current")
	got := FileIsModified(fs, "/etc/foo.conf", "different", false)
	if !got {
		t.Error("expected true when content differs")
	}
}

func TestFileIsModified_existsError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) {
		return false, errors.New("stat failed")
	}
	got := FileIsModified(fs, "/etc/foo.conf", "content", false)
	if got {
		t.Error("expected false when Exists errors")
	}
}

func TestFileIsModified_readError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) {
		return true, nil
	}
	// File not in Files map, so ReadFile returns error
	got := FileIsModified(fs, "/etc/foo.conf", "content", false)
	if got {
		t.Error("expected false when ReadFile errors")
	}
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

func TestWriteUserConfigs_modified(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	homeDir, err := UserHomeDir(testSys)
	if err != nil {
		t.Fatal(err)
	}
	// File exists with matching content => not modified => write proceeds
	expandedPath := filepath.Join(homeDir, ".config/foo.conf")
	fs.Files[expandedPath] = []byte("user content")

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
	if string(fs.Files[expandedPath]) != "user content" {
		t.Errorf("expected file content to be 'user content'")
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
