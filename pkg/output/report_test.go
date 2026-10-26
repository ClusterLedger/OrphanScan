package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestEstimatedMonthlyWasteMatchesReports(t *testing.T) {
	for _, tt := range []struct {
		name   string
		report WasteReport
		want   float64
	}{
		{name: "empty"},
		{
			name: "all resource types",
			report: WasteReport{
				Volumes: []scanner.OrphanedVolume{{SizeGB: 100}},
				EIPs:    []scanner.OrphanedEIP{{}, {}},
				LBs:     []scanner.IdleLoadBalancer{{Type: "CLB"}, {Type: "ALB"}, {Type: "NLB"}},
				PVCs:    []scanner.OrphanedPVC{{SizeGB: 12.5}},
			},
			want: 72.2,
		},
		{
			name:   "only available source findings",
			report: WasteReport{PVCs: []scanner.OrphanedPVC{{SizeGB: 25}}},
			want:   2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.EstimatedMonthlyWaste(); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("estimate = %v, want %v", got, tt.want)
			}
			var text, encoded bytes.Buffer
			if err := PrintText(&text, tt.report); err != nil {
				t.Fatal(err)
			}
			if tt.want != 0 && !strings.Contains(text.String(), fmt.Sprintf("Total Monthly Waste Detected: $%.2f", tt.want)) {
				t.Errorf("text total disagrees with estimate: %s", text.String())
			}
			if err := PrintJSON(&encoded, tt.report); err != nil {
				t.Fatal(err)
			}
			var report jsonReport
			if err := json.Unmarshal(encoded.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Total != tt.report.EstimatedMonthlyWaste() {
				t.Errorf("JSON total = %v, estimate = %v", report.Total, tt.report.EstimatedMonthlyWaste())
			}
		})
	}
}
