package self

import (
	"testing"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/testutil"
)

// mockFileInfo implements ports.FileInfo for testing ensureLink.
type mockFileInfo struct {
	isDir bool
}

func (m mockFileInfo) Name() string { return "" }
func (m mockFileInfo) Size() int64  { return 0 }
func (m mockFileInfo) IsDir() bool  { return m.isDir }

func TestEnsureLink_existsError(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.ExistsFunc = func(_ string) (bool, error) { return false, errMock }
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err == nil {
		t.Fatal("expected error from Exists")
	}
}

func TestEnsureLink_statError(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) { return nil, errMock }
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err == nil {
		t.Fatal("expected error from Stat")
	}
}

func TestEnsureLink_isDir(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) {
		return mockFileInfo{isDir: true}, nil
	}
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err == nil {
		t.Fatal("expected error when link is a directory")
	}
}

func TestEnsureLink_readlinkMatch(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) {
		return mockFileInfo{isDir: false}, nil
	}
	fs.ReadlinkFunc = func(_ string) (string, error) {
		return "/target", nil
	}
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestEnsureLink_readlinkMismatch(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) {
		return mockFileInfo{isDir: false}, nil
	}
	fs.ReadlinkFunc = func(_ string) (string, error) {
		return "/old-target", nil
	}
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestEnsureLink_readlinkError(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) {
		return mockFileInfo{isDir: false}, nil
	}
	fs.ReadlinkFunc = func(_ string) (string, error) {
		return "", errMock
	}
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err != nil {
		t.Errorf("expected nil on readlink error (removes and recreates), got %v", err)
	}
}

func TestEnsureLink_readlinkMismatchRemoveAllError(t *testing.T) {
	cfg := DefaultConfig()
	fs := testutil.NewMockFileSystem()
	fs.Files["/link"] = []byte{}
	fs.StatFunc = func(_ string) (ports.FileInfo, error) {
		return mockFileInfo{isDir: false}, nil
	}
	fs.ReadlinkFunc = func(_ string) (string, error) {
		return "/old-target", nil
	}
	fs.RemoveAllFunc = func(_ string) error { return errMock }
	u := NewUpdater(cfg, nil, fs, nil, nil, nil, false)
	if err := u.ensureLink("/target", "/link"); err == nil {
		t.Fatal("expected error from RemoveAll")
	}
}
