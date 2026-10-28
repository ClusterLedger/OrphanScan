package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestMaskingArbitraryNamesAndLateDashes(t *testing.T) {
	for _, name := range []string{"sensitivecustomer-", "verylongprivatebackend-1", "secret-prod-data", "https://private-cluster.invalid", "203.0.113.25", "eipalloc-secret-", "vol-1234", "private"} {
		if got := obfuscate(name); got != "***" {
			t.Errorf("%q exposed as %q", name, got)
		}
	}
}

func TestReportsAreDeterministicAndMasked(t *testing.T) {
	report := WasteReport{HasAWS: true, HasK8s: true,
		Volumes:   []scanner.OrphanedVolume{{VolumeID: "vol-0123456789abcdef0", Region: "eu-west-1", SizeGB: 100}, {VolumeID: "vol-0123456789abcdef1", Region: "us-east-1", SizeGB: 10}},
		LBs:       []scanner.IdleLoadBalancer{{Name: "sensitivecustomer-", Type: "ALB", Region: "us-east-1"}},
		LBReviews: []scanner.LoadBalancerReview{{IdleLoadBalancer: scanner.IdleLoadBalancer{Name: "unhealthy-private-alb", Type: "ALB", Region: "us-east-1"}, Reason: "registered_backends_unhealthy_or_unknown"}},
		PVCs:      []scanner.OrphanedPVC{{Name: "private-database-", Namespace: "private-namespace", SizeGB: 100}},
		Checks:    []scanner.Check{{Source: "aws", Resource: "ebs_volumes", Status: "complete"}},
	}
	var first, second, text bytes.Buffer
	if err := PrintJSON(&first, report); err != nil {
		t.Fatal(err)
	}
	report.Volumes[0], report.Volumes[1] = report.Volumes[1], report.Volumes[0]
	if err := PrintJSON(&second, report); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("completion order changed JSON")
	}
	if err := PrintText(&text, report); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"vol-0123456789abcdef0", "vol-0123456789abcdef1", "sensitivecustomer-", "unhealthy-private-alb", "private-database-", "private-namespace"} {
		if strings.Contains(first.String()+text.String(), private) {
			t.Errorf("identifier leaked: %s", private)
		}
	}
	if strings.Contains(first.String(), `"monthly_cost": 22`) {
		t.Fatal("review findings were priced")
	}
}

func TestIncompleteReportsNeverClaimCleanInfrastructure(t *testing.T) {
	for _, checks := range [][]scanner.Check{nil, {{Source: "aws", Status: "failed", Error: "permission denied"}}, {{Source: "aws", Status: "complete"}, {Source: "kubernetes", Status: "failed"}}} {
		var text bytes.Buffer
		if err := PrintText(&text, WasteReport{Checks: checks}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text.String(), "looks clean") || strings.Contains(text.String(), "Status: Complete") {
			t.Fatal(text.String())
		}
	}
}
