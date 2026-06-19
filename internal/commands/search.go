package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/coreservices"
)

type SearchCommand struct {
	listSvc *services.ListService
}

func NewSearchCommand(listSvc *services.ListService) *SearchCommand {
	return &SearchCommand{listSvc: listSvc}
}

func (c *SearchCommand) Name() string { return "search" }

func (c *SearchCommand) Usage() string { return "Search managed packages" }

func (c *SearchCommand) Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("search requires a query")
	}
	return c.listSvc.Search(ctx, strings.Join(args, " "))
}
