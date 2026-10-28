// This release preflight validates reporting settings without making requests.
package main

import (
	"fmt"
	"os"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
)

func main() {
	_, err := telemetry.NewClient(os.Getenv("ORPHANSCAN_POSTHOG_HOST"), os.Getenv("ORPHANSCAN_POSTHOG_PROJECT_TOKEN"))
	if err != nil {
		// NewClient returns fixed diagnostics and never echoes tokens or hosts.
		fmt.Fprintln(os.Stderr, "Invalid release metrics configuration:", err)
		os.Exit(1)
	}
}
