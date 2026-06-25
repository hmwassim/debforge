package ports

import "context"

type Spinner interface {
	Done()
	Fail()
	DoneWarn()
	DoneInfo()
	Pause()
	Resume()
	SetDesc(string)
}

type UI interface {
	Info(format string, args ...any)
	Success(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Prompt(format string, args ...any) bool
	PromptInput(defaultVal, format string, args ...any) string
	Spinner(ctx context.Context, desc string) Spinner
	SetYes(yes bool)
}
