package output

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	warnMu     sync.Mutex
	warnWriter io.Writer = os.Stderr
)

// SetWarnWriter redirects warning output to w. It is safe for concurrent use.
func SetWarnWriter(w io.Writer) {
	warnMu.Lock()
	defer warnMu.Unlock()
	warnWriter = w
}

// NewWarnFunc returns a warn function that writes [WARN] messages to w.
func NewWarnFunc(w io.Writer) func(string, ...any) {
	return func(format string, args ...any) {
		_, _ = fmt.Fprintf(w, "[WARN] "+format+"\n", args...)
	}
}

// Warn prints a warning message to the configured warn writer, prefixed with [WARN].
func Warn(format string, args ...any) {
	warnMu.Lock()
	w := warnWriter
	warnMu.Unlock()
	_, _ = fmt.Fprintf(w, "[WARN] "+format+"\n", args...)
}
