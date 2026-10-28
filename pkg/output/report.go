package output

import (
	"cmp"
	"slices"

	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// WasteReport preserves the original output API. New scan code can use report.WasteReport.
type WasteReport = report.WasteReport

// CostSummary preserves the original output API for the shared report model.
type CostSummary = report.CostSummary

// sortedReport makes a copy so formatting is deterministic and does not mutate callers.
func sortedReport(r WasteReport) WasteReport {
	r.Volumes = slices.Clone(r.Volumes)
	r.EIPs = slices.Clone(r.EIPs)
	r.LBs = slices.Clone(r.LBs)
	r.LBReviews = slices.Clone(r.LBReviews)
	r.PVCs = slices.Clone(r.PVCs)
	r.Checks = slices.Clone(r.Checks)
	slices.SortFunc(r.Volumes, func(a, b scanner.OrphanedVolume) int {
		return cmp.Or(cmp.Compare(a.Region, b.Region), cmp.Compare(a.VolumeID, b.VolumeID))
	})
	slices.SortFunc(r.EIPs, func(a, b scanner.OrphanedEIP) int {
		return cmp.Or(cmp.Compare(a.Region, b.Region), cmp.Compare(a.AllocationID, b.AllocationID))
	})
	slices.SortFunc(r.LBs, func(a, b scanner.IdleLoadBalancer) int {
		return cmp.Or(cmp.Compare(a.Region, b.Region), cmp.Compare(a.Type, b.Type), cmp.Compare(a.Name, b.Name))
	})
	slices.SortFunc(r.LBReviews, func(a, b scanner.LoadBalancerReview) int {
		return cmp.Or(cmp.Compare(a.Region, b.Region), cmp.Compare(a.Type, b.Type), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Reason, b.Reason))
	})
	slices.SortFunc(r.PVCs, func(a, b scanner.OrphanedPVC) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	slices.SortFunc(r.Checks, func(a, b scanner.Check) int {
		return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.Region, b.Region), cmp.Compare(a.Resource, b.Resource))
	})
	return r
}
