package output

import (
	"fmt"
	"io"
	"os"
)

// NewWarnFunc returns a warn function that writes [WARN] messages to w.
func NewWarnFunc(w io.Writer) func(string, ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(w, "[WARN] "+format+"\n", args...)
	}
}

// Warn prints a warning message to stderr, prefixed with [WARN].
var Warn = NewWarnFunc(os.Stderr)
