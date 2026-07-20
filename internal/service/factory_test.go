package service

import (
	"testing"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/testutil"
)

func TestServiceFactory_Install(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _ := newStateManagerForTest(t)

	f := NewServiceFactory(Deps{Reg: reg, InstReg: instReg, State: stateSvc})
	svc := f.Install()
	if svc == nil {
		t.Fatal("expected non-nil InstallService")
	}
}

func TestServiceFactory_Remove(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _ := newStateManagerForTest(t)

	f := NewServiceFactory(Deps{Reg: reg, InstReg: instReg, State: stateSvc})
	svc := f.Remove(testutil.NopPackageLister{})
	if svc == nil {
		t.Fatal("expected non-nil RemoveService")
	}
}

func TestServiceFactory_SharedDeps(t *testing.T) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()
	stateSvc, _ := newStateManagerForTest(t)

	deps := Deps{Reg: reg, InstReg: instReg, State: stateSvc}
	f := NewServiceFactory(deps)

	installSvc := f.Install()
	removeSvc := f.Remove(testutil.NopPackageLister{})

	// Both services should share the same state manager
	if installSvc.state != removeSvc.state {
		t.Error("expected both services to share the same StateManager")
	}
}
