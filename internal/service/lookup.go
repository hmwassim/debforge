package service

import (
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
)

// LookupPackage finds name in reg or returns a descriptive error. It is the
// single place that turns "package not found" into an error message, used
// by both InstallService and RemoveService (and by internal/self, which
// needs the same lookup when tearing down managed packages on self-remove).
func LookupPackage(reg *pkg.Registry, name string) (*pkg.Package, error) {
	p, ok := reg.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown package: %s", name)
	}
	return p, nil
}

// LookupInstaller finds the Installer registered for typ or returns a
// descriptive error, mirroring LookupPackage above.
func LookupInstaller(instReg *installer.Registry, typ pkg.Type) (installer.Installer, error) {
	inst, ok := instReg.Lookup(typ)
	if !ok {
		return nil, fmt.Errorf("no installer for type %s", typ)
	}
	return inst, nil
}
