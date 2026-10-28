package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestTextEstimateAvailability(t *testing.T) {
	for _, tt := range []struct {
		name             string
		report           WasteReport
		status, estimate string
	}{
		{"completed empty", WasteReport{Checks: []scanner.Check{{Source: "aws", Resource: "ebs_volumes", Status: "complete"}}}, "Complete", "$0.00/month"},
		{"discovery failed", WasteReport{Checks: []scanner.Check{{Source: "aws", Resource: "configuration", Status: "failed"}}}, "Failed", "Unavailable"},
		{"all APIs failed", WasteReport{Checks: []scanner.Check{{Source: "aws", Resource: "ebs_volumes", Status: "failed"}}}, "Failed", "Unavailable"},
		{"partially inspected", WasteReport{Volumes: []scanner.OrphanedVolume{{SizeGB: 100}}, Checks: []scanner.Check{{Source: "aws", Resource: "ebs_volumes", Status: "complete"}, {Source: "aws", Resource: "elastic_ips", Status: "failed"}}}, "Partial", "$8.00/month (available results only)"},
		{"partial zero", WasteReport{Checks: []scanner.Check{{Source: "aws", Resource: "elbv2_load_balancers", Status: "partial"}}}, "Partial", "$0.00/month (available results only)"},
		{"legacy findings", WasteReport{Volumes: []scanner.OrphanedVolume{{SizeGB: 100}}}, "Unknown", "$8.00/month (coverage unknown)"},
		{"requested without coverage", WasteReport{HasAWS: true}, "Unknown", "Unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var text bytes.Buffer
			if err := PrintText(&text, tt.report); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text.String(), "Status: "+tt.status) || !strings.Contains(text.String(), "AWS          "+tt.estimate+"\n") {
				t.Fatal(text.String())
			}
			if tt.estimate == "Unavailable" && strings.Contains(text.String(), "$0.00") {
				t.Fatal("failed or unattempted coverage was priced as zero")
			}
		})
	}
}

func TestVerboseCoverageAndGroupedIssues(t *testing.T) {
	message := "Permission denied.\nCheck the documented read-only permissions for this source."
	report := WasteReport{Checks: []scanner.Check{
		{Source: "aws", Region: "us-west-2", Resource: "elastic_ips", Status: "failed", Error: message},
		{Source: "aws", Region: "us-east-1", Resource: "ebs_volumes", Status: "failed", Error: message},
		{Source: "aws", Region: "us-west-2", Resource: "ebs_volumes", Status: "failed", Error: message},
	}}
	before, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, verbose := range []bool{false, true} {
		var text bytes.Buffer
		if err := PrintTextWithOptions(&text, report, TextOptions{Verbose: verbose}); err != nil {
			t.Fatal(err)
		}
		if strings.Count(text.String(), "Permission denied.") != 1 || !strings.Contains(text.String(), "Affects 3 checks across 2 regions.") {
			t.Fatal(text.String())
		}
		if strings.Contains(text.String(), "\nChecks\n") != verbose || strings.Contains(text.String(), "prices checked") != verbose {
			t.Fatalf("unexpected verbosity: %s", text.String())
		}
		if verbose && (!strings.Contains(text.String(), "Unattached EBS volumes") || !strings.Contains(text.String(), "us-west-2")) {
			t.Fatal("verbose coverage lost resource or region")
		}
		for _, unwanted := range []string{"ebs_volumes", "elastic_ips", "\x1b["} {
			if strings.Contains(text.String(), unwanted) {
				t.Fatalf("internal key or ANSI output: %s", text.String())
			}
		}
		after, err := json.Marshal(report)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("formatter mutated the report")
		}
	}

	var issues bytes.Buffer
	if err := PrintIssues(&issues, report.Checks); err != nil || !strings.Contains(issues.String(), "Check the documented read-only permissions") || strings.Count(issues.String(), "Permission denied.") != 1 {
		t.Fatalf("ungrouped JSON stderr diagnostics: err=%v text=%s", err, issues.String())
	}
	writeErr := errors.New("closed stderr")
	if err := PrintIssues(&failingWriter{failAt: 1, writeErr: writeErr}, report.Checks); !errors.Is(err, writeErr) {
		t.Fatal(err)
	}
}

func TestReviewFindingsRemainUnpriced(t *testing.T) {
	report := WasteReport{HasAWS: true, Checks: []scanner.Check{{Source: "aws", Region: "us-east-1", Resource: "elbv2_load_balancers", Status: "complete"}}, LBReviews: []scanner.LoadBalancerReview{{IdleLoadBalancer: scanner.IdleLoadBalancer{Name: "private-balancer", Type: "ALB", Region: "us-east-1"}}}}
	var text bytes.Buffer
	if err := PrintText(&text, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Unpriced reviews", "1 load balancer flagged for investigation", "Excluded from baseline estimates", "AWS          $0.00/month"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("missing %q: %s", want, text.String())
		}
	}
	if strings.Contains(text.String(), "private-balancer") || strings.Contains(text.String(), "$16.20") {
		t.Fatal(text.String())
	}
}

func TestInterruptedTextRetainsCompletedFindings(t *testing.T) {
	report := WasteReport{Volumes: []scanner.OrphanedVolume{{VolumeID: "private-volume", SizeGB: 100, Region: "us-east-1"}}, Checks: []scanner.Check{{Source: "aws", Region: "us-east-1", Resource: "ebs_volumes", Status: "complete"}}}
	var text bytes.Buffer
	if err := PrintTextWithOptions(&text, report, TextOptions{Interrupted: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "Status: Interrupted") || !strings.Contains(text.String(), "$8.00/month") || strings.Contains(text.String(), "private-volume") {
		t.Fatal(text.String())
	}
}
