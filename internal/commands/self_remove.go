package commands

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type SelfRemoveCommand struct {
	svc *services.SelfRemoveService
	ui  ports.UI
}

func NewSelfRemoveCommand(svc *services.SelfRemoveService, ui ports.UI) *SelfRemoveCommand {
	return &SelfRemoveCommand{svc: svc, ui: ui}
}

func (c *SelfRemoveCommand) Name() string { return "self-remove" }

func (c *SelfRemoveCommand) Usage() string { return "Remove debforge and all data" }

func (c *SelfRemoveCommand) Run(ctx context.Context, args []string) error {
	selection := strings.Join(args, " ")
	return c.svc.Run(ctx, selection)
}
