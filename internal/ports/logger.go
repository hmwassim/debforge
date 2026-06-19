package ports

type Logger interface {
	Info(format string, args ...any)
	Success(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Muted(format string, args ...any)
	Debug(format string, args ...any)
	Prompt(format string, args ...any) bool
}
