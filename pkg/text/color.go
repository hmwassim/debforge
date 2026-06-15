package text

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	gray   = "\033[90m"
)

const (
	frameColor   = bold + blue
	successColor = bold + green
	errorColor   = bold + red
)

func ansiPair(ok bool, code string) (string, string) {
	if ok {
		return code, reset
	}
	return "", ""
}
