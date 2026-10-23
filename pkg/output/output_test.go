package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestObfuscate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vol-0abc1234def56789", "vol-***6789"},
		{"eipalloc-abc123def456", "eipalloc-***f456"},
		{"short", "***"},
		{"i-1234abcd", "i-***abcd"},
		{"nodash12345678", "***5678"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Obfuscate(tt.input)
			if got != tt.want {
				t.Errorf("Obfuscate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrintText_WithFindings(t *testing.T) {
	report := WasteReport{
		Volumes: []scanner.OrphanedVolume{
			{VolumeID: "vol-abc123", Region: "us-east-1", SizeGB: 100, Age: 48 * time.Hour},
		},
		EIPs: []scanner.OrphanedEIP{
			{AllocationID: "eipalloc-xyz", PublicIP: "1.2.3.4", Region: "us-east-1"},
		},
	}

	var buf bytes.Buffer
	PrintText(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "OrphanScan") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "[Found]") {
		t.Error("missing [Found] line")
	}
	if !strings.Contains(out, "$") {
		t.Error("missing cost")
	}
}

func TestPrintText_ZeroWaste(t *testing.T) {
	var buf bytes.Buffer
	PrintText(&buf, WasteReport{})
	out := buf.String()

	if !strings.Contains(out, "Scan complete") {
		t.Error("missing scan complete message")
	}
}

func TestPrintJSON_Structure(t *testing.T) {
	report := WasteReport{
		Volumes: []scanner.OrphanedVolume{
			{VolumeID: "vol-abc1234567890def", Region: "us-east-1", SizeGB: 50, Age: 72 * time.Hour},
		},
	}

	var buf bytes.Buffer
	err := PrintJSON(&buf, report)
	if err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check total exists and is > 0
	total, ok := result["total_monthly_waste"].(float64)
	if !ok || total <= 0 {
		t.Errorf("expected positive total, got %v", result["total_monthly_waste"])
	}

	// Check volume ID is obfuscated
	volumes, ok := result["unattached_ebs_volumes"].([]interface{})
	if !ok || len(volumes) == 0 {
		t.Fatal("expected volumes in JSON")
	}
	vol := volumes[0].(map[string]interface{})
	volID := vol["volume_id"].(string)
	if !strings.Contains(volID, "***") {
		t.Errorf("volume_id should be obfuscated, got %q", volID)
	}
}
