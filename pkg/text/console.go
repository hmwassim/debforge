package text

import (
	"fmt"
	"io"
	"sync"
)

// consoleMu serializes all terminal output from the logger, spinner, and
// progress components so their frames and messages never interleave.
var consoleMu sync.Mutex

// ConsoleWritef acquires the console mutex and writes a formatted string to w.
// All text-component code that writes to stderr should use this function.
func ConsoleWritef(w io.Writer, format string, args ...interface{}) {
	consoleMu.Lock()
	fmt.Fprintf(w, format, args...)
	consoleMu.Unlock()
}
