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
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type testSender struct {
	costs     []float64
	err       error
	onCapture func()
}

func (*testSender) Configured() bool { return true }

func (s *testSender) Capture(_ context.Context, cost float64) error {
	s.costs = append(s.costs, cost)
	if s.onCapture != nil {
		s.onCapture()
	}
	return s.err
}

type failedOutput struct{ err error }

func (w failedOutput) Write([]byte) (int, error) { return 0, w.err }

func TestFinishScanReportsOnlyRenderedScans(t *testing.T) {
	writeErr := errors.New("stdout failed")
	for _, tt := range []struct {
		name, format string
		report       report.WasteReport
		writeErr     error
		deliveryErr  error
		wantCost     float64
	}{
		{name: "zero text", format: "text"},
		{name: "zero json", format: "json"},
		{
			name: "partial findings", format: "text",
			report: report.WasteReport{Volumes: []scanner.OrphanedVolume{{SizeGB: 100}}}, wantCost: 8,
		},
		{name: "text output failure", format: "text", writeErr: writeErr},
		{name: "json output failure", format: "json", writeErr: writeErr},
		{name: "offline is nonfatal", format: "json", deliveryErr: errors.New("offline")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.report.Checks = []scanner.Check{{Source: scanner.SourceAWS, Resource: "ebs_volumes", Status: scanner.StatusComplete}}
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
			err = finishScan(context.Background(), writer, tt.format, tt.report, reporter, nil, output.TextOptions{})
			if tt.writeErr != nil {
				if !errors.Is(err, tt.writeErr) || len(sender.costs) != 0 {
					t.Fatalf("render failure: err=%v, uploaded=%v", err, sender.costs)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(sender.costs) != 1 || sender.costs[0] != tt.wantCost {
				t.Fatalf("uploaded costs = %v, want [%v]", sender.costs, tt.wantCost)
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
	if err := finishScan(context.Background(), failedOutput{errors.New("closed")}, "text", report.WasteReport{}, reporter, nil, output.TextOptions{}); err == nil {
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
	err := runWithDependencies(context.Background(), Config{OutputFmt: "yaml"}, testRuntime(io.Discard, io.Discard), nil, dependencies{})
	if err == nil || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("expected output format error, got %v", err)
	}
}

func TestTelemetryPromptEligibility(t *testing.T) {
	for _, tt := range []struct {
		name, format, ci string
		terminal, want   bool
	}{
		{name: "interactive", format: "text", terminal: true, want: true},
		{name: "json", format: "json", terminal: true},
		{name: "CI true", format: "text", terminal: true, ci: "true"},
		{name: "CI false", format: "text", terminal: true, ci: "false"},
		{name: "redirected", format: "text"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := canPromptForTelemetry(tt.format, commandRuntime{ci: tt.ci, terminal: tt.terminal}); got != tt.want {
				t.Fatalf("prompt eligible=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewUsageReporterDoesNotTrack(t *testing.T) {
	runtime := testRuntime(io.Discard, io.Discard)
	runtime.configDir = t.TempDir()
	runtime.doNotTrack = true
	reporter, err := newUsageReporter(Config{Telemetry: "on"}, "https://us.i.posthog.com", "phc_test", runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(context.Background(), 99); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runtime.configDir, "orphanscan", "telemetry.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DO_NOT_TRACK changed the preference: %v", err)
	}
}

func TestNewUsageReporterUnconfiguredBuild(t *testing.T) {
	runtime := testRuntime(io.Discard, io.Discard)
	runtime.configDirErr = errors.New("unavailable config directory")
	reporter, err := newUsageReporter(Config{OutputFmt: "text"}, "", "", runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(context.Background(), 99); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsInvalidTelemetryMode(t *testing.T) {
	err := runWithDependencies(context.Background(), Config{OutputFmt: "text", Telemetry: "yes"}, testRuntime(io.Discard, io.Discard), nil, dependencies{})
	if err == nil || !strings.Contains(err.Error(), "--telemetry") {
		t.Fatalf("expected telemetry mode error, got %v", err)
	}
}
