package report_test

import (
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestPreferredMonthlyEstimate(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		result                     report.WasteReport
		aws, kubernetes, preferred float64
	}{
		{name: "empty"},
		{name: "AWS only", result: report.WasteReport{Volumes: []scanner.OrphanedVolume{{SizeGB: 100}}}, aws: 8, preferred: 8},
		{name: "Kubernetes only", result: report.WasteReport{PVCs: []scanner.OrphanedPVC{{SizeGB: 100}}}, kubernetes: 8, preferred: 8},
		{
			name: "overlapping sources stay separate",
			result: report.WasteReport{
				Volumes: []scanner.OrphanedVolume{{SizeGB: 100}},
				PVCs:    []scanner.OrphanedPVC{{SizeGB: 200}},
				Checks:  []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}},
			},
			aws: 8, kubernetes: 16, preferred: 8,
		},
		{
			name: "successful empty AWS check selects zero",
			result: report.WasteReport{
				PVCs:   []scanner.OrphanedPVC{{SizeGB: 200}},
				Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}},
			},
			kubernetes: 16,
		},
		{
			name: "partial AWS check is usable",
			result: report.WasteReport{
				PVCs:   []scanner.OrphanedPVC{{SizeGB: 200}},
				Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusPartial}},
			},
			kubernetes: 16,
		},
		{
			name: "failed AWS check falls back to Kubernetes",
			result: report.WasteReport{
				PVCs:   []scanner.OrphanedPVC{{SizeGB: 200}},
				Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusFailed}},
			},
			kubernetes: 16, preferred: 16,
		},
		{
			name: "existing AWS findings take precedence without a usable check",
			result: report.WasteReport{
				EIPs:   []scanner.OrphanedEIP{{}},
				PVCs:   []scanner.OrphanedPVC{{SizeGB: 200}},
				Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusFailed}},
			},
			aws: 3.60, kubernetes: 16, preferred: 3.60,
		},
		{
			name: "gateway zones priced and review findings excluded",
			result: report.WasteReport{
				LBs:       []scanner.IdleLoadBalancer{{Type: scanner.LBTypeGWLB, Zones: 2}},
				LBReviews: []scanner.LoadBalancerReview{{IdleLoadBalancer: scanner.IdleLoadBalancer{Type: scanner.LBTypeALB}}},
			},
			aws: 18, preferred: 18,
		},
		{
			name:       "per-finding rounding is preserved",
			result:     report.WasteReport{PVCs: []scanner.OrphanedPVC{{SizeGB: 1.234}, {SizeGB: 1.234}}},
			kubernetes: .20, preferred: .20,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			costs := tt.result.Costs()
			if costs.AWS != tt.aws || costs.Kubernetes != tt.kubernetes {
				t.Fatalf("costs=%+v, want AWS=%v Kubernetes=%v", costs, tt.aws, tt.kubernetes)
			}
			if got := tt.result.PreferredMonthlyEstimate(); got != tt.preferred {
				t.Fatalf("preferred estimate=%v, want %v", got, tt.preferred)
			}
			if got := tt.result.EstimatedMonthlyWaste(); got != tt.preferred {
				t.Fatalf("legacy estimate=%v, want %v", got, tt.preferred)
			}
		})
	}
}
