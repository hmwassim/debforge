package service

import "github.com/hmwassim/debforge/internal/ports"

// ServiceFactory constructs services from a single Deps value, eliminating
// the 4× identical Deps construction across command handlers.
type ServiceFactory struct {
	deps Deps
}

// NewServiceFactory returns a ServiceFactory that builds services from deps.
func NewServiceFactory(deps Deps) *ServiceFactory {
	return &ServiceFactory{deps: deps}
}

// Install returns an InstallService wired with the factory's deps.
func (f *ServiceFactory) Install() *InstallService {
	return NewInstallService(f.deps, NewResolver(f.deps.Reg))
}

// Remove returns a RemoveService wired with the factory's deps.
func (f *ServiceFactory) Remove(pkgLister ports.PackageLister) *RemoveService {
	return NewRemoveService(f.deps, pkgLister)
}
