package output

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestSeparateEstimatesAndTelemetry(t *testing.T) {
	report := WasteReport{
		HasAWS: true, HasK8s: true,
		Volumes: []scanner.OrphanedVolume{{SizeGB: 100}},
		EIPs:    []scanner.OrphanedEIP{{}, {}},
		LBs:     []scanner.IdleLoadBalancer{{Type: "CLB"}, {Type: "ALB"}, {Type: "NLB"}},
		PVCs:    []scanner.OrphanedPVC{{SizeGB: 100}},
		Checks: []scanner.Check{
			{Source: scanner.SourceAWS, Resource: "ebs_volumes", Status: scanner.StatusComplete},
			{Source: scanner.SourceKubernetes, Resource: "persistent_volume_claims", Status: scanner.StatusComplete},
		},
	}
	costs := report.Costs()
	if math.Abs(costs.AWS-65.60) > 1e-9 || costs.Kubernetes != 8 {
		t.Fatalf("costs = %+v", costs)
	}
	if report.EstimatedMonthlyWaste() != costs.AWS {
		t.Fatal("telemetry combined overlapping storage")
	}
	var text, encoded bytes.Buffer
	if err := PrintText(&text, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "AWS          $65.60/month") || !strings.Contains(text.String(), "Kubernetes   $8.00/month") {
		t.Fatal(text.String())
	}
	if err := PrintJSON(&encoded, report); err != nil {
		t.Fatal(err)
	}
	var decoded jsonReport
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 1 || decoded.AWSEstimate != costs.AWS || decoded.KubernetesEstimate != costs.Kubernetes {
		t.Fatalf("JSON = %s", encoded.String())
	}
	if strings.Contains(encoded.String(), "total_monthly_waste") {
		t.Fatal("combined total remains in JSON")
	}
	report.Volumes = nil
	report.EIPs = nil
	report.LBs = nil
	report.Checks[0].Status = scanner.StatusFailed
	if report.EstimatedMonthlyWaste() != 8 {
		t.Fatal("failed AWS check must fall back to Kubernetes")
	}
	report.Checks[0].Status = scanner.StatusComplete
	if report.EstimatedMonthlyWaste() != 0 {
		t.Fatal("successful empty AWS check must report zero, not a PVC sum")
	}
}
