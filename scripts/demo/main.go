// The demo renders synthetic findings through the production formatters.
// It never loads credentials, scans infrastructure, or reports telemetry.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func main() {
	format := flag.String("output", "text", "text or json")
	verbose := flag.Bool("verbose", false, "Show individual checks and reference pricing in text output")
	flag.Parse()
	report := report.WasteReport{
		HasAWS: true, HasK8s: true,
		Volumes:   []scanner.OrphanedVolume{{VolumeID: "vol-0123456789abcdef0", Region: "us-east-1", SizeGB: 100, Age: 72 * time.Hour}},
		EIPs:      []scanner.OrphanedEIP{{AllocationID: "eipalloc-0123456789abcdef0", PublicIP: "192.0.2.1", Region: "us-east-1"}},
		LBs:       []scanner.IdleLoadBalancer{{Name: "synthetic-empty-alb", Type: scanner.LBTypeALB, Region: "us-east-1"}},
		LBReviews: []scanner.LoadBalancerReview{{IdleLoadBalancer: scanner.IdleLoadBalancer{Name: "synthetic-unhealthy-alb", Type: scanner.LBTypeALB, Region: "us-east-1"}, Reason: "registered_backends_unhealthy_or_unknown"}},
		PVCs:      []scanner.OrphanedPVC{{Name: "synthetic-retained-data", Namespace: "synthetic", SizeGB: 100, Age: 72 * time.Hour}},
		Checks: []scanner.Check{
			{Source: "aws", Region: "us-east-1", Resource: "ebs_volumes", Status: "complete"},
			{Source: "aws", Region: "us-east-1", Resource: "elastic_ips", Status: "complete"},
			{Source: "aws", Region: "us-east-1", Resource: "classic_load_balancers", Status: "complete"},
			{Source: "aws", Region: "us-east-1", Resource: "elbv2_load_balancers", Status: "complete"},
			{Source: "kubernetes", Resource: "persistent_volume_claims", Status: "complete"},
		},
	}
	var err error
	switch *format {
	case "text":
		err = output.PrintTextWithOptions(os.Stdout, report, output.TextOptions{Verbose: *verbose})
	case "json":
		err = output.PrintJSON(os.Stdout, report)
	default:
		fmt.Fprintln(os.Stderr, "--output must be text or json")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not write demo report")
		os.Exit(1)
	}
}
