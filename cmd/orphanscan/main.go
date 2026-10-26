package main

import (
	"context"
	"fmt"
	"os"
)

var (
	version = "dev"
	// Set at build time using the public PostHog project token and ingestion host.
	telemetryProjectToken string
	telemetryHost         string
)

func main() {
	cfg := ParseFlags()

	if cfg.Version {
		fmt.Printf("OrphanScan %s\n", version)
		return
	}

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
}
