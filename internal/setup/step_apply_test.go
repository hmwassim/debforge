package setup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/textutil"
)

// ---- ExtrepoStep Apply tests ----------------------------------------------

func TestExtrepoStep_Apply_WritesConfig(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := fs.ReadFile("/etc/extrepo/config.yaml")
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if string(data) != extrepoConfigFiles[0].Content {
		t.Errorf("expected config content, got %q", string(data))
	}
	if cx.ConfigHashes["/etc/extrepo/config.yaml"] == "" {
		t.Error("hash should be recorded")
	}
}

func TestExtrepoStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = textutil.Sha256Hex([]byte("original baseline"))
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/extrepo/config.yaml.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestExtrepoStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/extrepo/config.yaml"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = textutil.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/extrepo/config.yaml")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
}

func TestExtrepoStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/extrepo/config.yaml")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}

// ---- ZramStep Apply tests -------------------------------------------------

func TestZramStep_Apply_WritesConfigAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/zram-generator.conf"); err != nil {
		t.Errorf("config file not written: %v", err)
	}
	if cx.ConfigHashes["/etc/systemd/zram-generator.conf"] == "" {
		t.Error("hash should be recorded")
	}
	var foundDaemon, foundStart bool
	for _, c := range cmds {
		if c == "systemctl daemon-reload" {
			foundDaemon = true
		}
		if c == "systemctl start systemd-zram-setup@zram0.service" {
			foundStart = true
		}
	}
	if !foundDaemon {
		t.Error("expected systemctl daemon-reload")
	}
	if !foundStart {
		t.Error("expected systemctl start systemd-zram-setup@zram0.service")
	}
}

func TestZramStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	userContent := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(userContent)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = textutil.Sha256Hex([]byte("original baseline"))
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) != userContent {
		t.Error("original should be untouched")
	}
	if _, err := fs.ReadFile("/etc/systemd/zram-generator.conf.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestZramStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(modified)
	original := `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = textutil.Sha256Hex([]byte(original))
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
	if cx.ConfigHashes["/etc/systemd/zram-generator.conf"] == "" {
		t.Error("hash should be recorded on skip")
	}
}

func TestZramStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/systemd/zram-generator.conf")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}

// ---- ResolvedStep Apply tests ---------------------------------------------

func TestResolvedStep_Apply_WritesConfigsAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		cmds = append(cmds, cmd)
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&ResolvedStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/resolved.conf.d/99-dot.conf"); err != nil {
		t.Errorf("99-dot.conf not written: %v", err)
	}
	if _, err := fs.ReadFile("/etc/NetworkManager/conf.d/10-dns.conf"); err != nil {
		t.Errorf("10-dns.conf not written: %v", err)
	}
	expected := []string{
		"ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf",
		"systemctl enable --now systemd-resolved",
		"nmcli general reload",
		"systemctl restart systemd-resolved",
		"resolvectl query debian.org",
	}
	for _, e := range expected {
		found := false
		for _, c := range cmds {
			if c == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %q not found in %v", e, cmds)
		}
	}
}

// ---- TimesyncdStep Apply tests --------------------------------------------

func TestTimesyncdStep_Apply_WritesConfigAndRunsServices(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		cmds = append(cmds, cmd)
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&TimesyncdStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"); err != nil {
		t.Errorf("config not written: %v", err)
	}
	var foundEnable, foundTimedate bool
	for _, c := range cmds {
		if c == "systemctl enable --now systemd-timesyncd" {
			foundEnable = true
		}
		if c == "timedatectl set-ntp true" {
			foundTimedate = true
		}
	}
	if !foundEnable {
		t.Error("expected systemctl enable --now systemd-timesyncd")
	}
	if !foundTimedate {
		t.Error("expected timedatectl set-ntp true")
	}
}

// ---- FontsStep Apply tests -------------------------------------------------

func TestFontsStep_Apply_WritesConfigAndRunsFcCache(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmds []string
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/fonts/local.conf"); err != nil {
		t.Errorf("config file not written: %v", err)
	}
	if cx.ConfigHashes["/etc/fonts/local.conf"] == "" {
		t.Error("hash should be recorded")
	}
	found := false
	for _, c := range cmds {
		if c == "fc-cache -f" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected fc-cache -f")
	}
}

func TestFontsStep_Apply_ConflictWritesSidecar(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = textutil.Sha256Hex([]byte("original baseline"))
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := fs.ReadFile("/etc/fonts/local.conf.debforge-new"); err != nil {
		t.Error("sidecar should exist")
	}
}

func TestFontsStep_Apply_DriftedSkipsWithoutForce(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/fonts/local.conf"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = textutil.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusDrifted}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/fonts/local.conf")
	if string(data) != modified {
		t.Error("user-modified file should not be overwritten")
	}
}

func TestFontsStep_Apply_ForceOverwrites(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("old content")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.Force = true
	if err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusConflict}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, _ := fs.ReadFile("/etc/fonts/local.conf")
	if string(data) == "old content" {
		t.Error("force should overwrite")
	}
}

// ---- ZramStep Apply error paths -------------------------------------------

func TestZramStep_Apply_DaemonReloadError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmdCount int
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmdCount++
		if cmdCount == 1 && name == "systemctl" && len(args) > 0 && args[0] == "daemon-reload" {
			return nil, nil, errors.New("daemon-reload failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestZramStep_Apply_StartServiceError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var cmdCount int
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		cmdCount++
		if cmdCount == 2 && name == "systemctl" && len(args) > 0 && args[0] == "start" {
			return nil, nil, errors.New("start failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&ZramStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- TimesyncdStep Apply error paths --------------------------------------

func TestTimesyncdStep_Apply_SystemctlError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "systemctl" {
			return nil, nil, errors.New("systemctl failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&TimesyncdStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTimesyncdStep_Apply_TimedatectlError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "timedatectl" {
			return nil, nil, errors.New("timedatectl failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&TimesyncdStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- ResolvedStep Apply error paths ---------------------------------------

func TestResolvedStep_Apply_SystemctlError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "systemctl" {
			return nil, nil, errors.New("systemctl failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&ResolvedStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolvedStep_Apply_NmcliError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "nmcli" {
			return nil, nil, errors.New("nmcli failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&ResolvedStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolvedStep_Apply_ResolvectlError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	var callCount int
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "resolvectl" {
			callCount++
			return nil, nil, errors.New("resolvectl failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	_ = callCount
	err := (&ResolvedStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error for resolvectl failure, got nil")
	}
}

// ---- ExtrepoStep Apply error paths ----------------------------------------

func TestExtrepoStep_Apply_WriteError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.WriteFileFunc = func(_ string, _ []byte, _ int) error {
		return errors.New("write denied")
	}
	runner := pkgCfgRunner("installed", nil, nil)
	cx := newPkgCfgCx(fs, runner)
	err := (&ExtrepoStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- FontsStep Apply error paths ------------------------------------------

func TestFontsStep_Apply_FcCacheError(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	runner := pkgCfgRunner("installed", nil, func(name string, args ...string) ([]byte, []byte, error) {
		if name == "fc-cache" {
			return nil, nil, errors.New("fc-cache failed")
		}
		return nil, nil, nil
	})
	cx := newPkgCfgCx(fs, runner)
	err := (&FontsStep{}).Apply(context.Background(), cx, CheckResult{Status: StatusMissing})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
