package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func newUsageReporter(cfg Config, host, token string, runtime commandRuntime) (*telemetry.Reporter, error) {
	client, err := telemetry.NewClient(host, token)
	if err != nil {
		return &telemetry.Reporter{}, err
	}
	if !client.Configured() && cfg.Telemetry == "" {
		return &telemetry.Reporter{}, nil
	}
	if runtime.configDirErr != nil {
		return &telemetry.Reporter{}, fmt.Errorf("preference could not be read or saved: %w", runtime.configDirErr)
	}
	return telemetry.NewReporter(client, telemetry.Options{
		Mode:           cfg.Telemetry,
		DoNotTrack:     runtime.doNotTrack,
		PreferencePath: filepath.Join(runtime.configDir, "orphanscan", "telemetry.json"),
		Interactive:    canPromptForTelemetry(cfg.OutputFmt, runtime),
		Input:          runtime.stdin,
		Output:         runtime.stderr,
	})
}

func canPromptForTelemetry(format string, runtime commandRuntime) bool {
	return format == "text" && runtime.ci == "" && runtime.terminal
}

func reportUsage(ctx context.Context, result report.WasteReport, usage *telemetry.Reporter, warn scanner.WarnFunc) {
	if ctx.Err() != nil {
		return
	}
	if err := usage.Report(ctx, result.PreferredMonthlyEstimate()); err != nil && warn != nil {
		warn("Usage reporting could not complete; no scan details were uploaded")
	}
}
