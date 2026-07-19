package setup

import (
	"context"
	"fmt"
)

// Runner executes a sequence of Steps, applying each when its Check
// reports a non-satisfied status.
type Runner struct {
	steps []Step
}

// NewRunner returns a Runner that executes the given steps in order.
func NewRunner(steps ...Step) *Runner {
	return &Runner{steps: steps}
}

// Run applies each step whose Check reports a missing, drifted, or
// conflicted status. It stops on the first error.
func (r *Runner) Run(ctx context.Context, cx *Context) error {
	for _, step := range r.steps {
		var result CheckResult
		if cx.Force {
			result = CheckResult{Status: StatusMissing, Summary: "force mode"}
		} else {
			result = step.Check(ctx, cx)
		}

		switch result.Status {
		case StatusSatisfied:
			cx.UI.Info("%s (exists already)", step.Name())

		case StatusDrifted:
			cx.UI.Warn("%s (modified by user)", step.Name())
			if err := step.Apply(ctx, cx, result); err != nil {
				return fmt.Errorf("%s: %w", step.Name(), err)
			}

		case StatusConflict:
			cx.UI.Warn("%s (modified by user)", step.Name())
			if err := step.Apply(ctx, cx, result); err != nil {
				return fmt.Errorf("%s: %w", step.Name(), err)
			}

		case StatusMissing:
			if err := step.Apply(ctx, cx, result); err != nil {
				return fmt.Errorf("%s: %w", step.Name(), err)
			}
			cx.UI.Success("%s", step.Name())

		case StatusError:
			return fmt.Errorf("%s: %s", step.Name(), result.Summary)
		}
	}
	return nil
}

// CheckAll runs every step's Check without applying anything.
// Used by the doctor command to display system status.
func (r *Runner) CheckAll(ctx context.Context, cx *Context) []CheckResult {
	results := make([]CheckResult, len(r.steps))
	for i, step := range r.steps {
		results[i] = step.Check(ctx, cx)
	}
	return results
}
