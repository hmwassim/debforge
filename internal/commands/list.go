package commands

import (
	"context"

	"github.com/hmwassim/debforge/internal/coreservices"
)

type ListCommand struct {
	svc *services.ListService
}

func NewListCommand(svc *services.ListService) *ListCommand {
	return &ListCommand{svc: svc}
}

func (c *ListCommand) Name() string { return "list" }

func (c *ListCommand) Usage() string { return "List managed packages" }

func (c *ListCommand) Run(ctx context.Context, args []string) error {
	return c.svc.Run(ctx)
}
