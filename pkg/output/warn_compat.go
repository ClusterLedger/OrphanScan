package output

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// These globals support the original warning API. The CLI uses NewWarnFunc
// instead so each invocation owns its warning destination.
var (
	warnMu     sync.Mutex
	warnWriter io.Writer = os.Stderr
)

// SetWarnWriter redirects the legacy Warn destination. It is safe for concurrent
// use. Use NewWarnFunc for warnings scoped to an individual invocation.
func SetWarnWriter(w io.Writer) {
	warnMu.Lock()
	defer warnMu.Unlock()
	warnWriter = w
}

// Warn preserves the original global warning API, prefixing messages with [WARN].
// Use NewWarnFunc to supply an explicit destination.
func Warn(format string, args ...any) {
	warnMu.Lock()
	defer warnMu.Unlock()
	_, _ = fmt.Fprintf(warnWriter, "[WARN] "+format+"\n", args...)
}
