package output

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestCleanupNoticeEligibility(t *testing.T) {
	for _, tt := range []struct {
		name    string
		awsSize int32
		pvcSize float64
		checks  []scanner.Check
		want    bool
	}{
		{name: "below threshold", pvcSize: 612.375, checks: []scanner.Check{{Source: scanner.SourceKubernetes, Status: scanner.StatusComplete}}},
		{name: "at threshold", pvcSize: 612.5, checks: []scanner.Check{{Source: scanner.SourceKubernetes, Status: scanner.StatusComplete}}},
		{name: "above threshold", pvcSize: 612.625, checks: []scanner.Check{{Source: scanner.SourceKubernetes, Status: scanner.StatusComplete}}, want: true},
		{name: "failed scan", pvcSize: 1000, checks: []scanner.Check{{Source: scanner.SourceKubernetes, Status: scanner.StatusFailed}}},
		{name: "partial scan", pvcSize: 1000, checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusFailed}, {Source: scanner.SourceKubernetes, Status: scanner.StatusComplete}}, want: true},
		{name: "empty usable AWS takes precedence", pvcSize: 1000, checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}, {Source: scanner.SourceKubernetes, Status: scanner.StatusComplete}}},
		{name: "AWS findings above threshold", awsSize: 1000, checks: []scanner.Check{{Source: scanner.SourceAWS, Status: scanner.StatusComplete}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := WasteReport{Checks: tt.checks, PVCs: []scanner.OrphanedPVC{{SizeGB: tt.pvcSize}}}
			if tt.awsSize > 0 {
				result.Volumes = []scanner.OrphanedVolume{{SizeGB: tt.awsSize}}
			}
			var buf bytes.Buffer
			if err := PrintText(&buf, result); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "> Automated cleanup (OrphanSweep) coming soon."); got != tt.want {
				t.Fatalf("cleanup notice=%v, want %v; report=%s", got, tt.want, buf.String())
			}
		})
	}
}

func TestFormattingLeavesCallerSlicesUnchanged(t *testing.T) {
	result := WasteReport{
		Volumes:   []scanner.OrphanedVolume{{VolumeID: "z"}, {VolumeID: "a"}},
		EIPs:      []scanner.OrphanedEIP{{AllocationID: "z"}, {AllocationID: "a"}},
		LBs:       []scanner.IdleLoadBalancer{{Name: "z"}, {Name: "a"}},
		LBReviews: []scanner.LoadBalancerReview{{IdleLoadBalancer: scanner.IdleLoadBalancer{Name: "z"}}, {IdleLoadBalancer: scanner.IdleLoadBalancer{Name: "a"}}},
		PVCs:      []scanner.OrphanedPVC{{Name: "z"}, {Name: "a"}},
		Checks:    []scanner.Check{{Source: "kubernetes", Status: scanner.StatusComplete}, {Source: "aws", Status: scanner.StatusComplete}},
	}
	before, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, render := range []func(io.Writer, WasteReport) error{PrintText, PrintJSON} {
		if err := render(io.Discard, result); err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("formatting reordered caller-owned data")
		}
	}
}
