package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestTelemetrySetupWarningsUseCommandStderr(t *testing.T) {
	for _, name := range []string{"invalid host", "config directory unavailable", "preference cannot be saved"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			runtime := testRuntime(&stdout, &stderr)
			runtime.configDir = t.TempDir()
			host := "https://us.i.posthog.com"
			switch name {
			case "invalid host":
				host = "http://invalid.example"
			case "config directory unavailable":
				runtime.configDirErr = errors.New("config directory unavailable")
			case "preference cannot be saved":
				runtime.configDir = filepath.Join(runtime.configDir, "file")
				if err := os.WriteFile(runtime.configDir, []byte("blocked"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			deps := successfulDependencies()
			deps.usage = func(cfg Config, runtime commandRuntime) (*telemetry.Reporter, error) {
				return newUsageReporter(cfg, host, "phc_test", runtime)
			}
			args := []string{"--sources=kubernetes", "--output=json", "--telemetry=off"}
			if code := runCommand(context.Background(), args, runtime, deps); code != 0 {
				t.Fatalf("optional telemetry setup failed the scan: code=%d stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "[WARN] Usage reporting disabled") || !json.Valid(stdout.Bytes()) {
				t.Fatalf("setup warning missed the command writer or corrupted JSON: stderr=%q stdout=%q", stderr.String(), stdout.String())
			}
		})
	}
}

func TestCommandRuntimeIsolation(t *testing.T) {
	for _, tt := range []struct {
		name, format, ci string
		terminal, prompt bool
	}{
		{name: "interactive opt out", format: "text", terminal: true, prompt: true},
		{name: "redirected text", format: "text"},
		{name: "JSON", format: "json", terminal: true},
		{name: "CI", format: "text", terminal: true, ci: "true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			runtime := testRuntime(&stdout, &stderr)
			runtime.stdin = strings.NewReader("no\n")
			runtime.terminal, runtime.ci = tt.terminal, tt.ci
			runtime.configDir = t.TempDir()
			deps := successfulDependencies()
			deps.usage = func(cfg Config, runtime commandRuntime) (*telemetry.Reporter, error) {
				return newUsageReporter(cfg, "https://us.i.posthog.com", "phc_test", runtime)
			}
			args := []string{"--sources=kubernetes", "--output=" + tt.format}
			if code := runCommand(context.Background(), args, runtime, deps); code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			if got := strings.Contains(stderr.String(), "Allow sharing"); got != tt.prompt {
				t.Fatalf("prompt=%v, want %v; stderr=%q", got, tt.prompt, stderr.String())
			}
			if strings.Contains(stdout.String(), "PostHog") || strings.Contains(stderr.String(), "[WARN]") {
				t.Fatalf("command output was contaminated: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if tt.format == "json" && !json.Valid(stdout.Bytes()) {
				t.Fatalf("invalid JSON: %s", stdout.String())
			}
			preference, err := telemetry.LoadPreference(filepath.Join(runtime.configDir, "orphanscan", "telemetry.json"))
			want := telemetry.Undecided
			if tt.prompt {
				want = telemetry.Disabled
			}
			if err != nil || preference != want {
				t.Fatalf("preference=%v, want %v, err=%v", preference, want, err)
			}
		})
	}
}

func TestFinishScanCompletionPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, status                                           string
		cancelled, expired, cancelDuringReporting, outputFails bool
		code, uploads                                          int
	}{
		{name: "complete", status: scanner.StatusComplete, uploads: 1},
		{name: "partial still reports", status: scanner.StatusPartial, code: 3, uploads: 1},
		{name: "failed never reports", status: scanner.StatusFailed, code: 1},
		{name: "cancelled complete", status: scanner.StatusComplete, cancelled: true, code: 130},
		{name: "cancellation precedes failed status", status: scanner.StatusFailed, cancelled: true, code: 130},
		{name: "deadline preserves complete results", status: scanner.StatusComplete, expired: true},
		{name: "deadline preserves partial results", status: scanner.StatusPartial, expired: true, code: 3},
		{name: "cancelled during reporting", status: scanner.StatusComplete, cancelDuringReporting: true, code: 130, uploads: 1},
		{name: "output failure precedes cancellation", status: scanner.StatusFailed, cancelled: true, outputFails: true, code: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tt.expired {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			}
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			sender := &testSender{}
			if tt.cancelDuringReporting {
				sender.onCapture = cancel
			}
			reporter, err := telemetry.NewReporter(sender, telemetry.Options{Mode: "on", PreferencePath: filepath.Join(t.TempDir(), "telemetry.json")})
			if err != nil {
				t.Fatal(err)
			}
			result := report.WasteReport{Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: tt.status}}}
			writer := io.Discard
			writeErr := errors.New("closed stdout")
			if tt.outputFails {
				writer = failedOutput{writeErr}
			}
			err = finishScan(ctx, writer, "json", result, reporter, nil, output.TextOptions{})
			code := 0
			if err != nil {
				var exit *exitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.code
			}
			if code != tt.code || len(sender.costs) != tt.uploads {
				t.Fatalf("exit=%d uploads=%v, want exit=%d uploads=%d", code, sender.costs, tt.code, tt.uploads)
			}
			if tt.outputFails && !errors.Is(err, writeErr) {
				t.Fatalf("lost output error: %v", err)
			}
		})
	}
}

func TestTelemetryPromptFailureWarnsWithoutFailingScan(t *testing.T) {
	sender := &testSender{}
	reporter, err := telemetry.NewReporter(sender, telemetry.Options{
		PreferencePath: filepath.Join(t.TempDir(), "telemetry.json"), Interactive: true,
		Input: strings.NewReader("no\n"), Output: failedOutput{errors.New("closed stderr")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var warnings bytes.Buffer
	result := report.WasteReport{Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}}}
	if err := finishScan(context.Background(), io.Discard, "json", result, reporter, output.NewWarnFunc(&warnings), output.TextOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings.String(), "[WARN] Usage reporting could not complete") || len(sender.costs) != 0 {
		t.Fatalf("prompt failure was mishandled: warnings=%q uploads=%v", warnings.String(), sender.costs)
	}
}
