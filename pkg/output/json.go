package output

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// --- JSON wire types ---

type jsonReport struct {
	SchemaVersion      int             `json:"schema_version"`
	Status             string          `json:"status"`
	Checks             []scanner.Check `json:"checks"`
	Currency           string          `json:"currency"`
	Pricing            jsonPricing     `json:"pricing"`
	AWSEstimate        float64         `json:"estimated_aws_monthly_cost_usd"`
	KubernetesEstimate float64         `json:"estimated_kubernetes_monthly_cost_usd"`
	LBReviews          []jsonLBReview  `json:"load_balancers_for_review,omitempty"`
	Volumes            []jsonVolume    `json:"unattached_ebs_volumes,omitempty"`
	EIPs               []jsonEIP       `json:"unassociated_eips,omitempty"`
	LBs                []jsonLB        `json:"idle_load_balancers,omitempty"`
	PVCs               []jsonPVC       `json:"unattached_pvcs,omitempty"`
}

type jsonPricing struct {
	Model           string `json:"model"`
	ReferenceRegion string `json:"reference_region"`
	ReferenceDate   string `json:"reference_date"`
	HoursPerMonth   int    `json:"hours_per_month"`
}

type jsonLBReview struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Region string `json:"region"`
	Reason string `json:"reason"`
}

type jsonVolume struct {
	VolumeID    string  `json:"volume_id"`
	Region      string  `json:"region"`
	SizeGB      int32   `json:"size_gb"`
	Age         string  `json:"age"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type jsonEIP struct {
	AllocationID string  `json:"allocation_id"`
	PublicIP     string  `json:"public_ip"`
	Region       string  `json:"region"`
	MonthlyCost  float64 `json:"monthly_cost"`
}

type jsonLB struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Region      string  `json:"region"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type jsonPVC struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	SizeGB      float64 `json:"size_gb"`
	MonthlyCost float64 `json:"monthly_cost"`
}

// --- public entry point ---

// PrintJSON encodes the waste report as pretty-printed JSON.
func PrintJSON(w io.Writer, report WasteReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildJSONReport(report))
}

// --- report assembly ---

func buildJSONReport(report WasteReport) jsonReport {
	report = sortedReport(report)
	volumes := mapVolumes(report.Volumes)
	eips := mapEIPs(report.EIPs)
	lbs := mapLBs(report.LBs)
	pvcs := mapPVCs(report.PVCs)
	reviews := make([]jsonLBReview, 0, len(report.LBReviews))
	for _, lb := range report.LBReviews {
		reviews = append(reviews, jsonLBReview{Name: obfuscate(lb.Name), Type: lb.Type, Region: lb.Region, Reason: lb.Reason})
	}
	checks := report.Checks
	if checks == nil {
		checks = []scanner.Check{}
	}
	costs := report.Costs()
	return jsonReport{
		SchemaVersion: 1, Status: report.Status(), Checks: checks, Currency: "USD",
		Pricing:     jsonPricing{Model: "baseline", ReferenceRegion: pricing.ReferenceRegion, ReferenceDate: pricing.ReferenceDate, HoursPerMonth: pricing.HoursPerMonth},
		AWSEstimate: costs.AWS, KubernetesEstimate: costs.Kubernetes,
		Volumes: volumes, EIPs: eips, LBs: lbs, PVCs: pvcs, LBReviews: reviews,
	}
}

// --- per-resource mappers ---

func mapVolumes(volumes []scanner.OrphanedVolume) []jsonVolume {
	out := make([]jsonVolume, 0, len(volumes))

	for _, v := range volumes {
		cost := pricing.EBSMonthlyCost(v.SizeGB)
		out = append(out, jsonVolume{
			VolumeID:    obfuscate(v.VolumeID),
			Region:      v.Region,
			SizeGB:      v.SizeGB,
			Age:         v.Age.Truncate(time.Second).String(),
			MonthlyCost: cost,
		})
	}

	return out
}

func mapEIPs(eips []scanner.OrphanedEIP) []jsonEIP {
	out := make([]jsonEIP, 0, len(eips))

	for _, e := range eips {
		cost := pricing.EIPMonthlyCost(1)
		out = append(out, jsonEIP{
			AllocationID: obfuscate(e.AllocationID),
			PublicIP:     obfuscate(e.PublicIP),
			Region:       e.Region,
			MonthlyCost:  cost,
		})
	}

	return out
}

func mapLBs(lbs []scanner.IdleLoadBalancer) []jsonLB {
	out := make([]jsonLB, 0, len(lbs))

	for _, lb := range lbs {
		cost := pricing.LoadBalancerMonthlyCost(lb)
		out = append(out, jsonLB{
			Name:        obfuscate(lb.Name),
			Type:        lb.Type,
			Region:      lb.Region,
			MonthlyCost: cost,
		})
	}

	return out
}

func mapPVCs(pvcs []scanner.OrphanedPVC) []jsonPVC {
	out := make([]jsonPVC, 0, len(pvcs))

	for _, pvc := range pvcs {
		cost := pricing.PVCMonthlyCost(pvc.SizeGB)
		out = append(out, jsonPVC{
			Name:        obfuscate(pvc.Name),
			Namespace:   obfuscate(pvc.Namespace),
			SizeGB:      pvc.SizeGB,
			MonthlyCost: cost,
		})
	}

	return out
}
