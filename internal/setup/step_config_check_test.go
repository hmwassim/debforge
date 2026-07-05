package setup

import (
	"context"
	"fmt"
	"testing"

	"github.com/hmwassim/debforge/internal/testutil"
	"github.com/hmwassim/debforge/internal/textutil"
)

// ---- ExtrepoStep tests ----------------------------------------------------

func TestExtrepoStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte(extrepoConfigFiles[0].Content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = textutil.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckMissing_Config(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("installed", nil, nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestExtrepoStep_CheckError(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", context.Canceled, nil))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = textutil.Sha256Hex([]byte(extrepoConfigFiles[0].Content))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestExtrepoStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/extrepo/config.yaml"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/extrepo/config.yaml"] = textutil.Sha256Hex([]byte("original baseline"))
	result := (&ExtrepoStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

// ---- ZramStep tests -------------------------------------------------------

func TestZramStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestZramStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(`[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = textutil.Sha256Hex([]byte(`[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestZramStep_CheckError(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", context.Canceled, nil))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusError {
		t.Errorf("expected error, got %v", result.Status)
	}
}

func TestZramStep_CheckDrifted(t *testing.T) {
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
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestZramStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/zram-generator.conf"] = []byte(modified)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/zram-generator.conf"] = textutil.Sha256Hex([]byte("original baseline"))
	result := (&ZramStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

// ---- ResolvedStep tests ---------------------------------------------------

func TestResolvedStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestResolvedStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no configs), got %v", result.Status)
	}
}

func TestResolvedStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	dotContent := `[Resolve]
DNS=1.1.1.2#security.cloudflare-dns.com 1.0.0.2#security.cloudflare-dns.com 2606:4700:4700::1112#security.cloudflare-dns.com 2606:4700:4700::1002#security.cloudflare-dns.com
FallbackDNS=9.9.9.9#dns.quad9.net 149.112.112.112#dns.quad9.net 2620:fe::fe#dns.quad9.net
DNSOverTLS=yes
DNSSEC=yes
DNSStubListener=yes
MulticastDNS=no
Cache=yes
Domains=~.
`
	nmContent := `[main]
dns=systemd-resolved
`
	fs.Files["/etc/systemd/resolved.conf.d/99-dot.conf"] = []byte(dotContent)
	fs.Files["/etc/NetworkManager/conf.d/10-dns.conf"] = []byte(nmContent)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/resolved.conf.d/99-dot.conf"] = textutil.Sha256Hex([]byte(dotContent))
	cx.ConfigHashes["/etc/NetworkManager/conf.d/10-dns.conf"] = textutil.Sha256Hex([]byte(nmContent))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestResolvedStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/resolved.conf.d/99-dot.conf"] = []byte("user changed")
	fs.Files["/etc/NetworkManager/conf.d/10-dns.conf"] = []byte(`[main]
dns=systemd-resolved
`)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/resolved.conf.d/99-dot.conf"] = textutil.Sha256Hex([]byte("user changed"))
	cx.ConfigHashes["/etc/NetworkManager/conf.d/10-dns.conf"] = textutil.Sha256Hex([]byte("original"))
	result := (&ResolvedStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

// ---- TimesyncdStep tests --------------------------------------------------

func TestTimesyncdStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	content := `[Time]
NTP=0.debian.pool.ntp.org 1.debian.pool.ntp.org
FallbackNTP=2.debian.pool.ntp.org 3.debian.pool.ntp.org
`
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte(content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = textutil.Sha256Hex([]byte(content))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	modified := "user modified content"
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte(modified)
	original := `[Time]
NTP=time.cloudflare.com
FallbackNTP=time.google.com 0.debian.pool.ntp.org 1.debian.pool.ntp.org 2.debian.pool.ntp.org 3.debian.pool.ntp.org
`
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = textutil.Sha256Hex([]byte(original))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestTimesyncdStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/systemd/timesyncd.conf.d/10-timesyncd.conf"] = textutil.Sha256Hex([]byte("original baseline"))
	result := (&TimesyncdStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

// ---- FontsStep tests ------------------------------------------------------

func TestFontsStep_CheckMissing_Package(t *testing.T) {
	cx := newPkgCfgCx(nil, pkgCfgRunner("", fmt.Errorf("exit 1"), nil))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing, got %v", result.Status)
	}
}

func TestFontsStep_CheckSatisfied(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte(fontsConfigFiles[0].Content)
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = textutil.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusSatisfied {
		t.Errorf("expected satisfied, got %v", result.Status)
	}
}

func TestFontsStep_CheckMissing_Config(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusMissing {
		t.Errorf("expected missing (no config), got %v", result.Status)
	}
}

func TestFontsStep_CheckDrifted(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = textutil.Sha256Hex([]byte(fontsConfigFiles[0].Content))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusDrifted {
		t.Errorf("expected drifted, got %v", result.Status)
	}
}

func TestFontsStep_CheckConflict(t *testing.T) {
	fs := testutil.NewMockFileSystem()
	fs.Files["/etc/fonts/local.conf"] = []byte("user modified")
	cx := newPkgCfgCx(fs, pkgCfgRunner("installed", nil, nil))
	cx.ConfigHashes["/etc/fonts/local.conf"] = textutil.Sha256Hex([]byte("original baseline"))
	result := (&FontsStep{}).Check(context.Background(), cx)
	if result.Status != StatusConflict {
		t.Errorf("expected conflict, got %v", result.Status)
	}
}

// ---- Step Name tests -------------------------------------------------------

func TestStepNames(t *testing.T) {
	tests := []struct {
		step Step
		want string
	}{
		{&ReposStep{}, "Configured Debian repositories"},
		{&I386Step{}, "Enabled i386 architecture"},
		{&UpgradeStep{}, "Upgraded system packages"},
		{&FirmwareStep{}, "Installed firmware"},
		{&DevtoolsStep{}, "Installed core development tools"},
		{&KernelStep{}, "Installed backported kernel"},
		{&ZramStep{}, "Configured zram swap"},
		{&ResolvedStep{}, "Configured DNS-over-TLS"},
		{&TimesyncdStep{}, "Configured NTP time sync"},
		{&ExtrepoStep{}, "Configured extrepo"},
		{&MesaStep{}, "Installed Mesa GPU drivers"},
		{&MultimediaStep{}, "Installed multimedia stack"},
		{&FontsStep{}, "Installed fonts"},
		{&DesktopStep{}, "Installed desktop tools"},
	}
	for _, tc := range tests {
		got := tc.step.Name()
		if got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.step, got, tc.want)
		}
	}
}
