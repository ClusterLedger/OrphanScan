package telemetry

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient("https://us.i.posthog.com", "phc_test")
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL + "/i/v0/e/"
	client.http.Transport = server.Client().Transport
	return client
}

func TestClientMinimalEvents(t *testing.T) {
	events := make(chan map[string]json.RawMessage, 2)
	client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/i/v0/e/" || req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s, content type %q", req.Method, req.URL.Path, req.Header.Get("Content-Type"))
		}
		if ua := req.Header.Get("User-Agent"); ua != "" {
			t.Errorf("expected empty user agent, got %q", ua)
		}
		var event map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&event); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		events <- event
		w.WriteHeader(http.StatusOK)
	})
	var previousID string
	for _, cost := range []float64{0, 12.34} {
		if err := client.Capture(context.Background(), cost); err != nil {
			t.Fatal(err)
		}
		event := <-events
		if len(event) != 5 {
			t.Fatalf("unexpected event fields: %v", event)
		}
		for _, key := range []string{"api_key", "event", "distinct_id", "timestamp", "properties"} {
			if _, ok := event[key]; !ok {
				t.Errorf("missing protocol field %s", key)
			}
		}
		if string(event["event"]) != `"scan_completed"` || string(event["api_key"]) != `"phc_test"` {
			t.Errorf("unexpected event or project token: %v", event)
		}
		var props map[string]json.RawMessage
		if err := json.Unmarshal(event["properties"], &props); err != nil {
			t.Fatal(err)
		}
		var gotCost float64
		if err := json.Unmarshal(props["estimated_monthly_waste_usd"], &gotCost); err != nil {
			t.Fatal(err)
		}
		if len(props) != 2 || string(props["$process_person_profile"]) != "false" || gotCost != cost {
			t.Errorf("unexpected metric or privacy fields: %s", event["properties"])
		}
		var id string
		if err := json.Unmarshal(event["distinct_id"], &id); err != nil {
			t.Fatal(err)
		}
		decoded, err := hex.DecodeString(id)
		if err != nil || len(decoded) != 16 || id == previousID {
			t.Errorf("expected a fresh random event ID, got %q", id)
		}
		previousID = id
		var timestamp time.Time
		if err := json.Unmarshal(event["timestamp"], &timestamp); err != nil || timestamp.IsZero() {
			t.Errorf("invalid event timestamp: %s", event["timestamp"])
		}
	}
}

func TestClientConfiguration(t *testing.T) {
	for _, tt := range []struct {
		host, token string
		wantError   bool
	}{
		{"", "", false},
		{"https://us.i.posthog.com", "phc_test", false},
		{"https://eu.i.posthog.com/", "phc_test", false},
		{"http://us.i.posthog.com", "phc_test", true},
		{"https://example.com", "phc_test", true},
		{"https://us.i.posthog.com", "phx_private", true},
		{"https://us.i.posthog.com", "phc_test\n", true},
	} {
		client, err := NewClient(tt.host, tt.token)
		if (err != nil) != tt.wantError {
			t.Errorf("NewClient(%q, token) error = %v", tt.host, err)
		}
		if err == nil && client.Configured() != (tt.token != "") {
			t.Errorf("unexpected configured state for %q", tt.host)
		}
		if err == nil && client.Configured() && client.http.Timeout != 2*time.Second {
			t.Errorf("reporting timeout = %v, want 2s", client.http.Timeout)
		}
	}
	client, err := NewClient("", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Capture(context.Background(), 1); err != nil {
		t.Fatalf("unconfigured build should skip capture: %v", err)
	}
}

func TestClientDeliveryFailures(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				if req.URL.Path == "/i/v0/e/" {
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
			if err := client.Capture(context.Background(), 1); err == nil {
				t.Fatal("expected delivery failure")
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("made %d requests; must not retry or follow redirects", got)
			}
		})
	}
}

func TestClientTimeoutAndCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	client := testClient(t, func(_ http.ResponseWriter, req *http.Request) {
		select {
		case <-req.Context().Done():
		case <-release:
		}
	})
	client.http.Timeout = 100 * time.Millisecond
	if err := client.Capture(context.Background(), 1); err == nil {
		t.Fatal("expected bounded HTTP timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Capture(ctx, 1); err == nil {
		t.Fatal("expected context cancellation")
	}
}

func TestClientRejectsInvalidCosts(t *testing.T) {
	var requests atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	for _, cost := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := client.Capture(context.Background(), cost); err == nil {
			t.Errorf("expected invalid cost error for %v", cost)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid costs must never be uploaded")
	}
}
