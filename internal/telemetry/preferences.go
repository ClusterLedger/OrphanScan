package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Preference records whether the user has explicitly allowed or declined reporting.
type Preference int

const (
	Undecided Preference = iota
	Enabled
	Disabled
)

// LoadPreference treats a missing file as undecided. Other failures require
// reporting to remain disabled rather than assuming consent.
func LoadPreference(path string) (Preference, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Undecided, nil
	}
	if err != nil {
		return Disabled, fmt.Errorf("read telemetry preference: %w", err)
	}
	var preference struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &preference); err != nil {
		return Disabled, fmt.Errorf("read telemetry preference: %w", err)
	}
	if preference.Enabled == nil {
		return Disabled, fmt.Errorf("telemetry preference is missing enabled")
	}
	if *preference.Enabled {
		return Enabled, nil
	}
	return Disabled, nil
}

// SavePreference atomically replaces the consent file without storing scan data.
func SavePreference(path string, enabled bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	file, err := os.CreateTemp(dir, ".telemetry-*")
	if err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	defer func() {
		// Close and remove are best-effort cleanup after the checked write/close.
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	if err := json.NewEncoder(file).Encode(struct {
		Enabled bool `json:"enabled"`
	}{enabled}); err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("save telemetry preference: %w", err)
	}
	return nil
}
