// Package report owns scan results and their baseline cost estimates.
package report

import (
	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// WasteReport aggregates findings and actual completion of the requested checks.
type WasteReport struct {
	Volumes   []scanner.OrphanedVolume
	EIPs      []scanner.OrphanedEIP
	LBs       []scanner.IdleLoadBalancer
	LBReviews []scanner.LoadBalancerReview
	PVCs      []scanner.OrphanedPVC
	Checks    []scanner.Check
	// HasK8s and HasAWS indicate requested sources, including failed ones.
	HasK8s bool
	HasAWS bool
}

// CostSummary keeps potentially overlapping AWS and Kubernetes costs separate.
type CostSummary struct {
	AWS        float64
	Kubernetes float64
}

// Costs never adds AWS and Kubernetes storage: they may describe the same disk.
func (r WasteReport) Costs() CostSummary {
	var costs CostSummary
	for _, volume := range r.Volumes {
		costs.AWS += pricing.EBSMonthlyCost(volume.SizeGB)
	}
	for _, lb := range r.LBs {
		costs.AWS += pricing.LoadBalancerMonthlyCost(lb)
	}
	costs.AWS += pricing.EIPMonthlyCost(len(r.EIPs))
	for _, pvc := range r.PVCs {
		costs.Kubernetes += pricing.PVCMonthlyCost(pvc.SizeGB)
	}
	costs.AWS = pricing.RoundUSD(costs.AWS)
	costs.Kubernetes = pricing.RoundUSD(costs.Kubernetes)
	return costs
}

// Status summarizes whether all requested checks completed.
func (r WasteReport) Status() string { return scanner.OverallStatus(r.Checks) }

// PreferredMonthlyEstimate selects one subtotal without adding overlapping costs.
// A usable AWS check takes precedence, even if its estimate is zero. Otherwise,
// existing AWS findings take precedence over the Kubernetes storage estimate.
func (r WasteReport) PreferredMonthlyEstimate() float64 {
	costs := r.Costs()
	for _, check := range r.Checks {
		if check.Source == scanner.SourceAWS && (check.Status == scanner.StatusComplete || check.Status == scanner.StatusPartial) {
			return costs.AWS
		}
	}
	if len(r.Volumes)+len(r.EIPs)+len(r.LBs) > 0 {
		return costs.AWS
	}
	return costs.Kubernetes
}

// EstimatedMonthlyWaste retains the historical single-estimate API.
// Use Costs for separate subtotals or PreferredMonthlyEstimate to select one.
func (r WasteReport) EstimatedMonthlyWaste() float64 {
	return r.PreferredMonthlyEstimate()
}
