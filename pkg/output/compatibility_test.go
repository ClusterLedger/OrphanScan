package output_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestExistingReportAPI(t *testing.T) {
	legacy := output.WasteReport{
		Volumes: []scanner.OrphanedVolume{{SizeGB: 100}},
		EIPs:    []scanner.OrphanedEIP{}, LBs: []scanner.IdleLoadBalancer{},
		LBReviews: []scanner.LoadBalancerReview{}, PVCs: []scanner.OrphanedPVC{},
		Checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}},
		HasAWS: true, HasK8s: true,
	}
	// Assignments and function types verify source compatibility for Go callers.
	var (
		shared      report.WasteReport
		oldReport   output.WasteReport
		costs       output.CostSummary
		sharedCosts report.CostSummary
	)
	shared = legacy
	oldReport = shared
	costs = shared.Costs()
	sharedCosts = costs
	if oldReport.Status() != "complete" || oldReport.EstimatedMonthlyWaste() != 8 || sharedCosts.AWS != 8 {
		t.Fatalf("legacy report contract changed: report=%+v costs=%+v", oldReport, sharedCosts)
	}
	for _, render := range []func(io.Writer, output.WasteReport) error{output.PrintText, output.PrintJSON} {
		var buf bytes.Buffer
		if err := render(&buf, oldReport); err != nil || buf.Len() == 0 {
			t.Fatalf("legacy formatter failed: err=%v", err)
		}
	}
}

func TestExistingWarningAPI(t *testing.T) {
	var legacy, scoped bytes.Buffer
	output.SetWarnWriter(&legacy)
	t.Cleanup(func() { output.SetWarnWriter(os.Stderr) })
	output.Warn("legacy %s", "message")
	output.NewWarnFunc(&scoped)("scoped %s", "message")
	if legacy.String() != "[WARN] legacy message\n" || scoped.String() != "[WARN] scoped message\n" {
		t.Fatalf("warning destinations changed: legacy=%q scoped=%q", legacy.String(), scoped.String())
	}
}
