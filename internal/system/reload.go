package system

import (
	"context"
	"fmt"
	"time"
)

// ReloadOrder is the order reload triggers run in: kernel modules first (so
// udev rules can select schedulers they provide), then systemd and sysctl,
// tmpfiles, udev rules (re-triggered so new rules apply without a reboot),
// and finally user-facing caches.
var ReloadOrder = []string{"modules", "systemd", "sysctl", "tmpfiles", "udev", "fontconfig", "desktop"}

var reloadCommands = map[string][][]string{
	"modules":  {{"systemctl", "restart", "systemd-modules-load.service"}},
	"systemd":  {{"systemctl", "daemon-reload"}},
	"sysctl":   {{"sysctl", "--system"}},
	"tmpfiles": {{"systemd-tmpfiles", "--create"}},
	"udev": {
		{"udevadm", "control", "--reload"},
		// Live zram devices (initstate=1) are skipped: they must never be
		// disturbed while in use as swap.
		{"udevadm", "trigger", "--action=change", "--attr-nomatch=initstate=1",
			"--subsystem-match=block", "--subsystem-match=sound", "--subsystem-match=power_supply",
			"--subsystem-match=powercap", "--subsystem-match=scsi_host", "--subsystem-match=pci",
			"--subsystem-match=misc", "--subsystem-match=rtc"},
		{"udevadm", "settle", "--timeout=30"},
	},
	"fontconfig": {{"fc-cache", "-f"}},
	"desktop":    {{"update-desktop-database", "-q", "/usr/local/share/applications"}},
}

// Reload runs the requested triggers in ReloadOrder. Failures are returned
// per trigger; they never stop the remaining triggers.
func Reload(ctx context.Context, r Runner, want map[string]bool) []error {
	var errs []error
	for _, name := range ReloadOrder {
		if !want[name] {
			continue
		}
		for _, c := range reloadCommands[name] {
			if _, err := r.Run(context.WithoutCancel(ctx), Cmd{Name: c[0], Args: c[1:], Timeout: 2 * time.Minute}); err != nil {
				errs = append(errs, fmt.Errorf("reload %s: %w", name, err))
				break
			}
		}
	}
	return errs
}
