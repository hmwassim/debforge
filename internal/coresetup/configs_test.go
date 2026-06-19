package coresetup

import (
	"strings"
	"testing"
)

func TestSourcesList(t *testing.T) {
	if !strings.Contains(SourcesList, "trixie") {
		t.Fatal("expected SourcesList to contain 'trixie'")
	}
	if !strings.Contains(SourcesList, "main") {
		t.Fatal("expected SourcesList to contain 'main'")
	}
	if !strings.Contains(SourcesList, "contrib") {
		t.Fatal("expected SourcesList to contain 'contrib'")
	}
	if !strings.Contains(SourcesList, "non-free-firmware") {
		t.Fatal("expected SourcesList to contain 'non-free-firmware'")
	}
}

func TestZramConfig(t *testing.T) {
	if !strings.Contains(ZramConfig, "zram0") {
		t.Fatal("expected ZramConfig to contain 'zram0'")
	}
	if !strings.Contains(ZramConfig, "zstd") {
		t.Fatal("expected ZramConfig to contain 'zstd'")
	}
}

func TestResolvedConfig(t *testing.T) {
	if !strings.Contains(ResolvedConfig, "cloudflare-dns.com") {
		t.Fatal("expected ResolvedConfig to contain cloudflare DNS")
	}
	if !strings.Contains(ResolvedConfig, "DNSOverTLS=yes") {
		t.Fatal("expected ResolvedConfig to enable DNS over TLS")
	}
}

func TestNmDNSConfig(t *testing.T) {
	if !strings.Contains(NmDNSConfig, "systemd-resolved") {
		t.Fatal("expected NmDNSConfig to use systemd-resolved")
	}
}

func TestTimesyncdConfig(t *testing.T) {
	if !strings.Contains(TimesyncdConfig, "time.cloudflare.com") {
		t.Fatal("expected TimesyncdConfig to contain NTP server")
	}
}

func TestAudioLimitsConfig(t *testing.T) {
	if !strings.Contains(AudioLimitsConfig, "rtprio") {
		t.Fatal("expected AudioLimitsConfig to contain rtprio")
	}
}

func TestFontConfig(t *testing.T) {
	if !strings.Contains(FontConfig, "Noto Color Emoji") {
		t.Fatal("expected FontConfig to contain emoji config")
	}
	if !strings.Contains(FontConfig, "Cousine") {
		t.Fatal("expected FontConfig to contain Cousine (monospace)")
	}
}
