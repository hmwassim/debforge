package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
)

// ---- DesktopStep tests -----------------------------------------------------

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

// ---- DesktopStep Apply error paths ----------------------------------------

func TestDesktopStep_Apply_FlatpakError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{}
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "flatpak" {
			return nil, nil, errors.New("flatpak failed")
		}
		return nil, nil, nil
	})
	cx := desktopCxWithFs(fs, runner, "")
	err := (&DesktopStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error for flatpak failure, got nil")
	}
}

// ---- DesktopStep bashrc error paths ----------------------------------------

func TestDesktopStep_Check_BashrcNotReadable(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/home/user/.config/bashrc.d"] = []byte{}
	// No .bashrc file — ReadFile returns error
	cx := desktopCxWithFs(fs, pkgCfgRunner("installed", nil, nil), "")
	result := (&DesktopStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing for unreadable bashrc, got %v", result.Status)
	}
}
