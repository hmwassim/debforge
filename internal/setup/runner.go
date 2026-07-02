package setup

import (
	"context"
	"fmt"
)

type Runner struct {
	steps []Step
}

func NewRunner(steps ...Step) *Runner {
	return &Runner{steps: steps}
}

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
			cx.UI.Info("✓ %s", step.Name())

		case StatusMissing, StatusDrifted, StatusConflict:
			cx.UI.Info("→ %s (%s)", step.Name(), result.Summary)
			if err := step.Apply(ctx, cx, result); err != nil {
				return fmt.Errorf("%s: %w", step.Name(), err)
			}

		case StatusError:
			return fmt.Errorf("%s: check failed: %s", step.Name(), result.Summary)
		}
	}
	return nil
}

func (r *Runner) CheckAll(ctx context.Context, cx *Context) []CheckResult {
	results := make([]CheckResult, len(r.steps))
	for i, step := range r.steps {
		results[i] = step.Check(ctx, cx)
	}
	return results
}
