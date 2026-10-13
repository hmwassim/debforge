package system

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// portableChassis are the SMBIOS chassis types of battery-powered machines:
// portable, laptop, notebook, hand held, sub notebook, tablet, convertible
// and detachable.
var portableChassis = []string{"8", "9", "10", "11", "14", "30", "31", "32"}

// DetectChassis returns "laptop" or "desktop", reading sysfs under root
// ("" in production). The SMBIOS chassis type decides; a system battery
// also counts as a laptop, for firmware that reports a wrong type.
// Batteries of peripherals (mice, headsets) report scope "Device" and are
// ignored.
func DetectChassis(root string) string {
	if b, err := os.ReadFile(root + "/sys/class/dmi/id/chassis_type"); err == nil &&
		slices.Contains(portableChassis, strings.TrimSpace(string(b))) {
		return "laptop"
	}
	m, _ := filepath.Glob(root + "/sys/class/power_supply/*/type")
	for _, f := range m {
		b, err := os.ReadFile(f)
		if err != nil || strings.TrimSpace(string(b)) != "Battery" {
			continue
		}
		scope, _ := os.ReadFile(filepath.Join(filepath.Dir(f), "scope"))
		if strings.TrimSpace(string(scope)) != "Device" {
			return "laptop"
		}
	}
	return "desktop"
}
