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

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type testSender struct {
	costs []float64
	err   error
}

func (*testSender) Configured() bool { return true }

func (s *testSender) Capture(_ context.Context, cost float64) error {
	s.costs = append(s.costs, cost)
	return s.err
}

type failedOutput struct{ err error }

func (w failedOutput) Write([]byte) (int, error) { return 0, w.err }

func TestFinishScanReportsOnlyRenderedScans(t *testing.T) {
	writeErr := errors.New("stdout failed")
	for _, tt := range []struct {
		name, format string
		report       output.WasteReport
		writeErr     error
		deliveryErr  error
	}{
		{name: "zero text", format: "text"},
		{name: "zero json", format: "json"},
		{
			name: "partial findings", format: "text",
			report: output.WasteReport{Volumes: []scanner.OrphanedVolume{{SizeGB: 100}}},
		},
		{name: "text output failure", format: "text", writeErr: writeErr},
		{name: "json output failure", format: "json", writeErr: writeErr},
		{name: "offline is nonfatal", format: "json", deliveryErr: errors.New("offline")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sender := &testSender{err: tt.deliveryErr}
			reporter, err := telemetry.NewReporter(sender, telemetry.Options{
				Mode: "on", PreferencePath: filepath.Join(t.TempDir(), "telemetry.json"),
			})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			var writer io.Writer = &buf
			if tt.writeErr != nil {
				writer = failedOutput{tt.writeErr}
			}
			err = finishScan(context.Background(), writer, tt.format, tt.report, reporter)
			if tt.writeErr != nil {
				if !errors.Is(err, tt.writeErr) || len(sender.costs) != 0 {
					t.Fatalf("render failure: err=%v, uploaded=%v", err, sender.costs)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(sender.costs) != 1 || sender.costs[0] != tt.report.EstimatedMonthlyWaste() {
				t.Fatalf("uploaded costs = %v, want [%v]", sender.costs, tt.report.EstimatedMonthlyWaste())
			}
			if tt.format == "json" && !json.Valid(buf.Bytes()) {
				t.Errorf("reporting altered JSON output: %s", buf.Bytes())
			}
		})
	}
}

func TestFinishScanDoesNotAskBeforeOutputSucceeds(t *testing.T) {
	sender := &testSender{}
	var prompt bytes.Buffer
	path := filepath.Join(t.TempDir(), "telemetry.json")
	reporter, err := telemetry.NewReporter(sender, telemetry.Options{
		PreferencePath: path, Interactive: true, Input: strings.NewReader("yes\n"), Output: &prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := finishScan(context.Background(), failedOutput{errors.New("closed")}, "text", output.WasteReport{}, reporter); err == nil {
		t.Fatal("expected output failure")
	}
	if prompt.Len() != 0 || len(sender.costs) != 0 {
		t.Fatal("failed output must not prompt or report")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("saved consent before successful output: %v", err)
	}
}

func TestRunRejectsInvalidOutputFormat(t *testing.T) {
	err := run(context.Background(), Config{OutputFmt: "yaml"})
	if err == nil || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("expected output format error, got %v", err)
	}
}

func TestTelemetryPromptEligibility(t *testing.T) {
	if canPromptForTelemetry("json", "") {
		t.Error("JSON must never prompt")
	}
	if canPromptForTelemetry("text", "true") {
		t.Error("CI must never prompt")
	}
	if canPromptForTelemetry("text", "false") {
		t.Error("a set CI variable must suppress prompting")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = stdout
		_ = reader.Close()
		_ = writer.Close()
	})
	if canPromptForTelemetry("text", "") {
		t.Error("redirected output must never prompt")
	}
}

func TestNewUsageReporterDoesNotTrack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	reporter := newUsageReporter(Config{Telemetry: "on"})
	if err := reporter.Report(context.Background(), 99); err != nil {
		t.Fatal(err)
	}
}

func TestNewUsageReporterUnconfiguredBuild(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	token, host := telemetryProjectToken, telemetryHost
	telemetryProjectToken, telemetryHost = "", ""
	t.Cleanup(func() { telemetryProjectToken, telemetryHost = token, host })
	reporter := newUsageReporter(Config{OutputFmt: "text"})
	if err := reporter.Report(context.Background(), 99); err != nil {
		t.Fatal(err)
	}
}
