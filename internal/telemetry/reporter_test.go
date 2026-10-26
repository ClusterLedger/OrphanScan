package telemetry

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingSender struct {
	configured bool
	costs      []float64
	err        error
}

func (s *recordingSender) Configured() bool { return s.configured }

func (s *recordingSender) Capture(_ context.Context, cost float64) error {
	s.costs = append(s.costs, cost)
	return s.err
}

func TestReporterConsent(t *testing.T) {
	tests := []struct {
		name         string
		initial      Preference
		mode         string
		answer       string
		interactive  bool
		doNotTrack   bool
		unconfigured bool
		deliveryErr  error
		wantPrompt   bool
		wantReports  int
		wantSaved    Preference
	}{
		{name: "yes", interactive: true, answer: "yes\n", wantPrompt: true, wantReports: 1, wantSaved: Enabled},
		{name: "no", interactive: true, answer: "n\n", wantPrompt: true, wantSaved: Disabled},
		{name: "blank defaults no", interactive: true, answer: "\n", wantPrompt: true, wantSaved: Disabled},
		{name: "EOF defaults no", interactive: true, wantPrompt: true, wantSaved: Disabled},
		{name: "invalid answer defaults no", interactive: true, answer: "maybe\n", wantPrompt: true, wantSaved: Disabled},
		{name: "noninteractive defaults off", wantSaved: Undecided},
		{name: "remembered yes", initial: Enabled, interactive: true, wantReports: 1, wantSaved: Enabled},
		{name: "remembered yes in scripts", initial: Enabled, wantReports: 1, wantSaved: Enabled},
		{name: "remembered no", initial: Disabled, interactive: true, wantSaved: Disabled},
		{name: "enable overrides no", initial: Disabled, mode: "on", interactive: true, wantReports: 1, wantSaved: Enabled},
		{name: "disable overrides yes", initial: Enabled, mode: "off", interactive: true, wantSaved: Disabled},
		{name: "do not track keeps preference", initial: Enabled, mode: "off", doNotTrack: true, interactive: true, wantSaved: Enabled},
		{name: "unconfigured does not prompt", unconfigured: true, interactive: true, wantSaved: Undecided},
		{name: "unconfigured saves explicit choice", unconfigured: true, mode: "on", wantSaved: Enabled},
		{name: "delivery errors are ignored", initial: Enabled, deliveryErr: errors.New("offline"), wantReports: 1, wantSaved: Enabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "orphanscan", "telemetry.json")
			if tt.initial != Undecided {
				if err := SavePreference(path, tt.initial == Enabled); err != nil {
					t.Fatal(err)
				}
			}
			sender := &recordingSender{configured: !tt.unconfigured, err: tt.deliveryErr}
			var out bytes.Buffer
			r, err := NewReporter(sender, Options{
				Mode: tt.mode, DoNotTrack: tt.doNotTrack, Interactive: tt.interactive,
				PreferencePath: path, Input: strings.NewReader(tt.answer), Output: &out,
			})
			if err != nil {
				t.Fatal(err)
			}
			// Even an accidental second call must not count the scan twice.
			for range 2 {
				if err := r.Report(context.Background(), 12.34); err != nil {
					t.Fatal(err)
				}
			}
			if got := len(sender.costs); got != tt.wantReports {
				t.Errorf("reported %d times, want %d", got, tt.wantReports)
			}
			if len(sender.costs) != 0 && sender.costs[0] != 12.34 {
				t.Errorf("reported cost = %v, want 12.34", sender.costs[0])
			}
			if tt.wantPrompt && out.String() != invitation {
				t.Errorf("invitation = %q, want %q", out.String(), invitation)
			}
			if !tt.wantPrompt && out.Len() != 0 {
				t.Errorf("unexpected prompt: %q", out.String())
			}
			saved, err := LoadPreference(path)
			if err != nil || saved != tt.wantSaved {
				t.Errorf("saved preference = %v, %v; want %v", saved, err, tt.wantSaved)
			}
			if saved != Undecided {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				want := "{\"enabled\":false}\n"
				if saved == Enabled {
					want = "{\"enabled\":true}\n"
				}
				if string(data) != want {
					t.Errorf("preference file contains extra data: %q", data)
				}
			}
		})
	}
}

func TestReporterInvalidPreferences(t *testing.T) {
	for _, data := range []string{"broken", "{}", "null", `{"enabled":null}`, `{"enabled":"yes"}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "telemetry.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			sender := &recordingSender{configured: true}
			var out bytes.Buffer
			r, err := NewReporter(sender, Options{
				PreferencePath: path, Interactive: true, Input: strings.NewReader("y\n"), Output: &out,
			})
			if err == nil {
				t.Fatal("expected invalid preference error")
			}
			if err := r.Report(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if len(sender.costs) != 0 || out.Len() != 0 {
				t.Fatal("invalid preferences must neither prompt nor report")
			}
		})
	}
}

func TestReporterPreferenceFailures(t *testing.T) {
	for _, mode := range []string{"", "on", "off"} {
		t.Run("read or save "+mode, func(t *testing.T) {
			// A directory at the file path fails on all platforms, including as root.
			path := t.TempDir()
			sender := &recordingSender{configured: true}
			r, err := NewReporter(sender, Options{Mode: mode, PreferencePath: path})
			if err == nil {
				t.Fatal("expected preference error")
			}
			if err := r.Report(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if len(sender.costs) != 0 {
				t.Fatal("reported despite preference failure")
			}
		})
	}
	t.Run("saving prompt answer fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.json")
		sender := &recordingSender{configured: true}
		var out bytes.Buffer
		r, err := NewReporter(sender, Options{
			PreferencePath: path, Interactive: true, Input: strings.NewReader("yes\n"), Output: &out,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := r.Report(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "not saved") {
			t.Fatalf("expected unsaved preference error, got %v", err)
		}
		if len(sender.costs) != 0 {
			t.Fatal("reported despite unsaved consent")
		}
	})
}

type brokenIO struct{}

func (brokenIO) Read([]byte) (int, error)  { return 0, errors.New("read failed") }
func (brokenIO) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestReporterPromptIOFailures(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "telemetry.json")
		sender := &recordingSender{configured: true}
		options := Options{PreferencePath: path, Interactive: true, Input: strings.NewReader("y\n"), Output: brokenIO{}}
		if failRead {
			options.Input = brokenIO{}
			options.Output = &bytes.Buffer{}
		}
		r, err := NewReporter(sender, options)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Report(context.Background(), 1); err == nil {
			t.Fatal("expected prompt I/O error")
		}
		if len(sender.costs) != 0 {
			t.Fatal("reported despite prompt failure")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("saved choice after prompt failure: %v", err)
		}
	}
}

func TestReporterRejectsInvalidMode(t *testing.T) {
	r, err := NewReporter(&recordingSender{configured: true}, Options{Mode: "yes"})
	if err == nil {
		t.Fatal("expected invalid flag error")
	}
	if err := r.Report(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestReporterNilIsNoop(t *testing.T) {
	var r *Reporter
	if err := r.Report(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestReporterZeroValueIsNoop(t *testing.T) {
	r := &Reporter{}
	if err := r.Report(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}
