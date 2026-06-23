package installer

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func StubError(typ string) error {
	return fmt.Errorf("%s installer: not implemented", typ)
}

type StubInstaller struct {
	Type string
}

func NewStubInstaller(typ string) *StubInstaller {
	return &StubInstaller{Type: typ}
}

func (s *StubInstaller) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return StubError(s.Type)
}

func (s *StubInstaller) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return StubError(s.Type)
}
