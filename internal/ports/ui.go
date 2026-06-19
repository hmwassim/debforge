package ports

import (
	"context"
	"io"
)

type Progress interface {
	io.Writer
	Done()
	Fail()
}

type Spinner interface {
	Done()
	Fail()
	Pause()
	Resume()
	SetDesc(desc string)
	DoneWarn()
}

type UI interface {
	Logger
	Spinner(ctx context.Context, description string) Spinner
	Progress(total int64, description string) Progress
	PromptInput(format string, args ...any) string
}
