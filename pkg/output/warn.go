package output

import (
	"fmt"
	"os"
)

// Warn prints a warning message to stderr, prefixed with [WARN].
func Warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}
