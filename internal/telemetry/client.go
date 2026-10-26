package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Client sends minimal, personless scan events to the maintainer's PostHog project.
type Client struct {
	endpoint string
	token    string
	http     *http.Client
}

// NewClient only accepts public project tokens and PostHog Cloud ingestion hosts.
// A missing token returns an unconfigured client, which never sends requests.
func NewClient(host, projectToken string) (*Client, error) {
	if projectToken == "" {
		return &Client{}, nil
	}
	if !strings.HasPrefix(projectToken, "phc_") || strings.ContainsAny(projectToken, " \t\r\n") {
		return nil, fmt.Errorf("telemetry requires a public PostHog project token (phc_)")
	}
	host = strings.TrimSuffix(host, "/")
	if host != "https://us.i.posthog.com" && host != "https://eu.i.posthog.com" {
		return nil, fmt.Errorf("telemetry requires a PostHog Cloud HTTPS ingestion host")
	}
	return &Client{
		endpoint: host + "/i/v0/e/",
		token:    projectToken,
		http: &http.Client{
			Timeout: 2 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Configured reports whether this build has a reporting destination.
func (c *Client) Configured() bool {
	return c != nil && c.endpoint != "" && c.token != ""
}

// Capture sends a single scan_completed event. The only application metric
// in the payload is the estimated monthly waste.
func (c *Client) Capture(ctx context.Context, estimatedMonthlyWaste float64) error {
	if !c.Configured() {
		return nil
	}
	if estimatedMonthlyWaste < 0 || math.IsNaN(estimatedMonthlyWaste) || math.IsInf(estimatedMonthlyWaste, 0) {
		return fmt.Errorf("invalid estimated monthly waste")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Errorf("generate telemetry event ID: %w", err)
	}
	payload := struct {
		APIKey     string    `json:"api_key"`
		Event      string    `json:"event"`
		DistinctID string    `json:"distinct_id"`
		Timestamp  time.Time `json:"timestamp"`
		Properties struct {
			EstimatedMonthlyWasteUSD float64 `json:"estimated_monthly_waste_usd"`
			ProcessPersonProfile     bool    `json:"$process_person_profile"`
		} `json:"properties"`
	}{
		APIKey:     c.token,
		Event:      "scan_completed",
		DistinctID: hex.EncodeToString(id[:]),
		Timestamp:  time.Now().UTC(),
	}
	payload.Properties.EstimatedMonthlyWasteUSD = estimatedMonthlyWaste
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode telemetry event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telemetry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OrphanScan")
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send telemetry event: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telemetry endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}
