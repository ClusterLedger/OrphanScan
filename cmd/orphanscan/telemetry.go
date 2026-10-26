package main

import (
	"os"
	"path/filepath"

	"golang.org/x/term"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/output"
)

func newUsageReporter(cfg Config, host, token string) *telemetry.Reporter {
	doNotTrack := os.Getenv("DO_NOT_TRACK") == "1"
	client, err := telemetry.NewClient(host, token)
	if err != nil {
		output.Warn("Usage reporting disabled: %v", err)
		return &telemetry.Reporter{}
	}
	if !client.Configured() && cfg.Telemetry == "" {
		return &telemetry.Reporter{}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		output.Warn("Usage reporting disabled; preference could not be read or saved: %v", err)
		return &telemetry.Reporter{}
	}
	reporter, err := telemetry.NewReporter(client, telemetry.Options{
		Mode:           cfg.Telemetry,
		DoNotTrack:     doNotTrack,
		PreferencePath: filepath.Join(configDir, "orphanscan", "telemetry.json"),
		Interactive:    canPromptForTelemetry(cfg.OutputFmt, os.Getenv("CI")),
		Input:          os.Stdin,
		Output:         os.Stderr,
	})
	if err != nil {
		output.Warn("Usage reporting disabled: %v", err)
	}
	return reporter
}

func canPromptForTelemetry(format, ci string) bool {
	return format == "text" && ci == "" &&
		term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		term.IsTerminal(int(os.Stderr.Fd()))
}
