package commands

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type SelfUpdateCommand struct {
	svc *services.UpdateService
	ui  ports.UI
}

func NewSelfUpdateCommand(svc *services.UpdateService, ui ports.UI) *SelfUpdateCommand {
	return &SelfUpdateCommand{svc: svc, ui: ui}
}

func (c *SelfUpdateCommand) Name() string { return "self-update" }

func (c *SelfUpdateCommand) Usage() string { return "Install or update debforge itself" }

func (c *SelfUpdateCommand) Run(ctx context.Context, args []string) error {
	return c.svc.Run(ctx)
}
