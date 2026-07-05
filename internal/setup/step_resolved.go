package setup

import (
	"context"
	"fmt"
	"time"

	"github.com/hmwassim/debforge/internal/aptpty"
)

var resolvedPackages = []string{"systemd-resolved"}

var resolvedConfigFiles = []ConfigFile{
	{
		Path: "/etc/systemd/resolved.conf.d/99-dot.conf",
		Content: `[Resolve]
DNS=1.1.1.2#security.cloudflare-dns.com 1.0.0.2#security.cloudflare-dns.com 2606:4700:4700::1112#security.cloudflare-dns.com 2606:4700:4700::1002#security.cloudflare-dns.com
FallbackDNS=9.9.9.9#dns.quad9.net 149.112.112.112#dns.quad9.net 2620:fe::fe#dns.quad9.net
DNSOverTLS=yes
DNSSEC=yes
DNSStubListener=yes
MulticastDNS=no
Cache=yes
Domains=~.
`,
	},
	{
		Path: "/etc/NetworkManager/conf.d/10-dns.conf",
		Content: `[main]
dns=systemd-resolved
`,
	},
}

type ResolvedStep struct{}

func (s *ResolvedStep) Name() string {
	return "Configured DNS-over-TLS"
}

func (s *ResolvedStep) Check(ctx context.Context, cx *Context) CheckResult {
	ok, err := allInstalled(ctx, cx.Runner, resolvedPackages)
	if err != nil {
		return CheckResult{Status: StatusError, Summary: fmt.Sprintf("dpkg query failed: %s", err)}
	}
	if !ok {
		return CheckResult{Status: StatusMissing, Summary: "systemd-resolved not installed"}
	}
	return checkConfigFiles(cx, resolvedConfigFiles)
}

func (s *ResolvedStep) Apply(ctx context.Context, cx *Context, result CheckResult) error {
	initialDesc := "Configuring systemd-resolved"
	if result.Status == StatusMissing {
		initialDesc = "Installing systemd-resolved"
	}
	spinner := cx.UI.Spinner(ctx, initialDesc)
	defer spinner.Stop()

	if result.Status == StatusMissing {
		if err := aptpty.RunInstall(ctx, cx.Runner, resolvedPackages, spinner); err != nil {
			return err
		}
		spinner.SetDesc("Configuring systemd-resolved")
	}

	if err := processConfigFiles(cx, resolvedConfigFiles, result); err != nil {
		return err
	}

	if _, _, err := cx.Runner.Run(ctx, "ln", "-sf", "/run/systemd/resolve/stub-resolv.conf", "/etc/resolv.conf"); err != nil {
		return fmt.Errorf("link resolv.conf: %w", err)
	}
	if _, _, err := cx.Runner.Run(ctx, "systemctl", "enable", "--now", "systemd-resolved"); err != nil {
		return fmt.Errorf("enable systemd-resolved: %w", err)
	}
	if _, _, err := cx.Runner.Run(ctx, "nmcli", "general", "reload"); err != nil {
		return fmt.Errorf("nmcli reload: %w", err)
	}
	if _, _, err := cx.Runner.Run(ctx, "systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("restart systemd-resolved: %w", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for i := 0; i < 15; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		_, _, err := cx.Runner.Run(ctx, "resolvectl", "query", "debian.org")
		if err == nil {
			break
		}
	}

	return nil
}
