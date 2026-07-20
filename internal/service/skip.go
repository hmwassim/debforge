package service

import "github.com/hmwassim/debforge/internal/domain/pkg"

// skipCheck describes which skip condition applies to a dependency.
// The caller must interpret the result — cases that need disk
// verification (skipCheckDisk) require an installer.CheckInstalled
// call before the final skip/install decision can be made.
type skipCheck int

const (
	skipCheckNone    skipCheck = iota // no skip condition; proceed with install
	skipCheckDisk                     // package may exist on disk; verify via dpkg-query
	skipCheckVariant                  // variant is __skip__; skip unconditionally
)

// evalSkipCheck determines which skip condition, if any, applies to dep.
// It is a pure function — no I/O, no mutation, fully deterministic.
//
// The three conditions are evaluated in the same order as the original
// shouldSkip method so that the combined function (evalSkipCheck +
// caller-side CheckInstalled) is behaviorally identical.
func evalSkipCheck(dep *pkg.Package, exists, rerun bool) skipCheck {
	if dep.SkipUpdate && !dep.ForceInstall && exists {
		return skipCheckDisk
	}
	if !rerun && exists {
		return skipCheckDisk
	}
	if dep.Apt != nil && dep.Apt.Variant == "__skip__" {
		return skipCheckVariant
	}
	return skipCheckNone
}
