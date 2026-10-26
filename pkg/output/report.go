package output

import (
	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// EstimatedMonthlyWaste returns the baseline monthly waste estimate in USD.
// It describes the available findings, which may come from a partial scan.
func (r WasteReport) EstimatedMonthlyWaste() float64 {
	return sumCost(r.Volumes, func(v scanner.OrphanedVolume) float64 {
		return pricing.EBSMonthlyCost(v.SizeGB)
	}) + sumCost(r.LBs, func(lb scanner.IdleLoadBalancer) float64 {
		return pricing.LBMonthlyCost(lb.Type)
	}) + pricing.EIPMonthlyCost(len(r.EIPs)) + sumCost(r.PVCs, func(pvc scanner.OrphanedPVC) float64 {
		return pricing.PVCMonthlyCost(pvc.SizeGB)
	})
}
