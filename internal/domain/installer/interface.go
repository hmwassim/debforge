package installer

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer handles installation and removal of a specific package type.
type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Remove(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
}

// BatchInstaller is an optional interface that installers can implement to
// support batch installation. When present, the service layer calls Prepare
// on each package first, then runs a single apt-get install for all of them,
// then calls Finalize on each package. This eliminates redundant PTY sessions
// and pre-flight checks when installing multiple packages.
type BatchInstaller interface {
	// Prepare does per-package setup (GPU check, conflicts, version check,
	// variant selection, download) without running the main apt-get install.
	// For deb packages this downloads the .deb files into a temp dir.
	// For apt packages this handles conflict removal, extrepo, pre-install
	// scripts, version check, variant selection, and backports.
	// Returns BatchArgs with the apt-get arguments for the batch call.
	// A zero-value BatchArgs with nil error means the package was skipped.
	Prepare(ctx context.Context, p *pkg.Package, spinner ports.Spinner) (BatchArgs, error)

	// Finalize runs post-install work after the batch apt-get install.
	// For apt: version recording, config writing, post-install scripts.
	// For deb: post-install scripts, temp dir cleanup.
	Finalize(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error
}

// BatchArgs holds the apt-get arguments collected by a BatchInstaller's
// Prepare method. The service layer aggregates these into a single call.
type BatchArgs struct {
	Skipped  bool     // true when the package doesn't need installation
	AptPkgs  []string // package names for apt-get install
	DebPaths []string // .deb file paths for apt-get install
}

// AssertType returns an error if typ does not match expected, using name in
// the error message to identify the installer.
func AssertType(typ, expected pkg.Type, name string) error {
	if typ != expected {
		return fmt.Errorf("%s installer called for type %s", name, typ)
	}
	return nil
}
