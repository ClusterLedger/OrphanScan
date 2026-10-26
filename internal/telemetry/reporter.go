package telemetry

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const invitation = "Help us maintain OrphanScan by sharing scan metrics via PostHog.\n" +
	"Includes the number of scans and estimated monthly waste.\n" +
	"Allow sharing for this and future scans? [y/N] "

// Sender is the reporting destination; it is never called before consent.
type Sender interface {
	Configured() bool
	Capture(context.Context, float64) error
}

// Options describes the user's preference and whether asking is appropriate.
type Options struct {
	Mode           string
	DoNotTrack     bool
	Interactive    bool
	PreferencePath string
	Input          io.Reader
	Output         io.Writer
}

// Reporter applies consent to a single CLI invocation.
type Reporter struct {
	sender   Sender
	options  Options
	enabled  bool
	prompt   bool
	finished bool
}

const (
	ModeOn  = "on"
	ModeOff = "off"
)

// ValidateMode accepts the default saved preference or an explicit on/off flag.
func ValidateMode(mode string) error {
	if mode != "" && mode != ModeOn && mode != ModeOff {
		return fmt.Errorf("--telemetry must be on or off")
	}
	return nil
}

// NewReporter saves explicit preference changes before scanning. On preference
// errors it returns a disabled reporter and an error for the CLI to warn about.
func NewReporter(sender Sender, options Options) (*Reporter, error) {
	r := &Reporter{sender: sender, options: options}
	if err := ValidateMode(options.Mode); err != nil {
		return r, err
	}
	if options.DoNotTrack {
		return r, nil
	}
	if options.Mode != "" {
		enabled := options.Mode == ModeOn
		if err := SavePreference(options.PreferencePath, enabled); err != nil {
			return r, fmt.Errorf("preference was not saved; reporting disabled for this run: %w", err)
		}
		r.enabled = enabled
		return r, nil
	}
	if sender == nil || !sender.Configured() {
		return r, nil
	}
	preference, err := LoadPreference(options.PreferencePath)
	if err != nil {
		return r, err
	}
	r.enabled = preference == Enabled
	r.prompt = preference == Undecided && options.Interactive && options.Input != nil && options.Output != nil
	return r, nil
}

// Report is called only after a report was rendered successfully. Preference and
// prompt failures can be surfaced locally; delivery failures are best-effort.
func (r *Reporter) Report(ctx context.Context, estimatedMonthlyWaste float64) error {
	if r == nil || r.finished {
		return nil
	}
	r.finished = true
	if r.options.DoNotTrack || r.sender == nil || !r.sender.Configured() {
		return nil
	}
	if r.prompt {
		if _, err := fmt.Fprint(r.options.Output, invitation); err != nil {
			return fmt.Errorf("show telemetry invitation: %w", err)
		}
		answer, err := bufio.NewReader(r.options.Input).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read telemetry choice: %w", err)
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		enabled := answer == "y" || answer == "yes"
		if err := SavePreference(r.options.PreferencePath, enabled); err != nil {
			return fmt.Errorf("preference was not saved; reporting disabled for this run: %w", err)
		}
		r.enabled = enabled
	}
	if r.enabled {
		// Reporting must not change scan output or exit status when offline,
		// rate-limited, or unable to reach the analytics service.
		_ = r.sender.Capture(ctx, estimatedMonthlyWaste)
	}
	return nil
}
