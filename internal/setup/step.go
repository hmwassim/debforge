package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

type StepStatus int

const (
	StatusSatisfied StepStatus = iota
	StatusMissing
	StatusDrifted
	StatusConflict
	StatusError
)

type CheckResult struct {
	Status  StepStatus
	Summary string
}

type Context struct {
	Runner ports.CommandRunner
	Fsys   ports.FileSystem
	Sys    ports.System
	UI     ports.UI
	Force  bool

	ConfigHashes map[string]string
}

type Step interface {
	Name() string
	Check(ctx context.Context, cx *Context) CheckResult
	Apply(ctx context.Context, cx *Context, result CheckResult) error
}

type RollbackStep interface {
	Step
	Rollback(ctx context.Context, cx *Context) error
}
