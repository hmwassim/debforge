// Package setup implements the system provisioning workflow that
// configures repos, firmware, kernel, desktop, and other system
// components on first install.
package setup

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

// StepStatus describes the outcome of a Step's Check method.
type StepStatus int

const (
	// StatusSatisfied means the step's prerequisite is already met.
	StatusSatisfied StepStatus = iota
	// StatusMissing means the step has not been applied yet.
	StatusMissing
	// StatusDrifted means the user has modified a file the step manages.
	StatusDrifted
	// StatusConflict means a file exists that conflicts with the step.
	StatusConflict
	// StatusError means the check itself failed.
	StatusError
)

// CheckResult is the result returned by Step.Check.
type CheckResult struct {
	Status  StepStatus
	Summary string
}

// Context carries the shared dependencies available to every Step.
type Context struct {
	Runner ports.CommandRunner
	Fsys   ports.FileSystem
	Sys    ports.System
	UI     ports.UI
	Force  bool

	ConfigHashes map[string]string
}

// Step is a single provisioning step (repos, firmware, desktop, etc.).
type Step interface {
	Name() string
	Check(ctx context.Context, cx *Context) CheckResult
	Apply(ctx context.Context, cx *Context, result CheckResult) error
}
