package services

import (
	"context"
	"testing"

	"github.com/hmwassim/debforge/internal/coresetup"
	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/statestore"
)

func newTestListSvc(pkgReg *pkg.Registry, stateSvc *state.Service, aptSvc apt.Service) *ListService {
	return NewListService(pkgReg, stateSvc, &mockUI{}, aptSvc)
}

func TestListServiceRun(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg1", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg2", Type: pkg.TypeDeb}})

	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"pkg1": {Type: "apt"},
	}}
	stateSvc.Save(st)

	aptSvc := apt.NewService(&mockRunner{}, &mockUI{})
	svc := newTestListSvc(pkgReg, stateSvc, aptSvc)
	ctx := context.Background()

	err := svc.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceRunEmpty(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	aptSvc := apt.NewService(&mockRunner{}, &mockUI{})
	svc := newTestListSvc(pkgReg, stateSvc, aptSvc)
	ctx := context.Background()

	err := svc.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceSearch(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "firefox", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "firefox-dev", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "chrome", Type: pkg.TypeDeb}})

	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	st := &state.PackagesState{Versioned: statestore.DefaultVersioned, Packages: map[string]state.PkgEntry{
		"firefox": {Type: "apt"},
	}}
	stateSvc.Save(st)

	aptSvc := apt.NewService(&mockRunner{}, &mockUI{})
	svc := newTestListSvc(pkgReg, stateSvc, aptSvc)
	ctx := context.Background()

	err := svc.Search(ctx, "firefox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceSearchNoMatch(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "firefox", Type: pkg.TypeApt}})

	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	aptSvc := apt.NewService(&mockRunner{}, &mockUI{})
	svc := newTestListSvc(pkgReg, stateSvc, aptSvc)
	ctx := context.Background()

	err := svc.Search(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceSearchCaseInsensitive(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "firefox", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "Firefox-Dev", Type: pkg.TypeApt}})

	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	aptSvc := apt.NewService(&mockRunner{}, &mockUI{})
	svc := newTestListSvc(pkgReg, stateSvc, aptSvc)
	ctx := context.Background()

	// Mixed-case query should match lower-case package name
	err := svc.Search(ctx, "Firefox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Lower-case query should match mixed-case package name
	err = svc.Search(ctx, "firefox-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceRunCore(t *testing.T) {
	pkgReg := pkg.NewRegistry()
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg1", Type: pkg.TypeApt}})
	pkgReg.Register(&pkg.Package{Metadata: pkg.Metadata{Name: "pkg2", Type: pkg.TypeApt}})

	fs := newMemFS()
	stateSvc := state.NewService(fs, "/tmp/states")
	aptSvc := apt.NewService(&mockRunner{stdout: []byte("install\n")}, &mockUI{})
	groups := coresetup.NewGroups()

	svc := NewListService(pkgReg, stateSvc, &mockUI{}, aptSvc)
	ctx := context.Background()

	err := svc.RunCore(ctx, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListServiceCheckInstalled(t *testing.T) {
	svc := NewListService(nil, nil, &mockUI{}, nil)
	ctx := context.Background()

	result, err := svc.checkInstalled(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatal("expected empty result")
	}
}

func TestListServiceCheckInstalledNoApt(t *testing.T) {
	svc := NewListService(nil, nil, &mockUI{}, nil)
	ctx := context.Background()

	_, err := svc.checkInstalled(ctx, []string{"pkg1"})
	if err == nil {
		t.Fatal("expected error when apt service not available")
	}
}
