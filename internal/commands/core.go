package commands

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/coresetup"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type CoreCommand struct {
	setupSvc *services.SetupService
	listSvc  *services.ListService
	groups   *coresetup.Groups
	ui       ports.UI
}

func NewCoreCommand(setupSvc *services.SetupService, listSvc *services.ListService, groups *coresetup.Groups, ui ports.UI) *CoreCommand {
	return &CoreCommand{setupSvc: setupSvc, listSvc: listSvc, groups: groups, ui: ui}
}

func (c *CoreCommand) Name() string { return "core" }

func (c *CoreCommand) Usage() string { return "Set up core packages and configs" }

func (c *CoreCommand) Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("core requires a subcommand: setup, list")
	}
	rest := args[1:]
	switch args[0] {
	case "setup":
		force := false
		for _, a := range rest {
			if a == "-f" || a == "--force" {
				force = true
				break
			}
		}
		return c.setupSvc.Run(ctx, force)
	case "list":
		return c.listSvc.RunCore(ctx, c.groups)
	default:
		return fmt.Errorf("unknown core subcommand: %s", args[0])
	}
}
