package output

import (
	"fmt"
	"io"
	"sync"
)

// NewWarnFunc returns a warn function that writes [WARN] messages to w.
func NewWarnFunc(w io.Writer) func(string, ...any) {
	var mu sync.Mutex
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(w, "[WARN] "+format+"\n", args...)
	}
}
