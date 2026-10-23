package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// WasteReport aggregates all detected orphaned/idle cloud resources.
type WasteReport struct {
	Volumes []scanner.OrphanedVolume
	EIPs    []scanner.OrphanedEIP
	LBs     []scanner.IdleLoadBalancer
	PVCs    []scanner.OrphanedPVC
}

// PrintText writes a human-readable waste audit to w.
func PrintText(w io.Writer, report WasteReport) {
	fmt.Fprintln(w, "=== OrphanScan: Waste Audit ===")
	fmt.Fprintln(w, "Scanning local kubeconfig and AWS environment...")
	fmt.Fprintln(w)

	total := printVolumes(w, report.Volumes) +
		printLBs(w, report.LBs) +
		printEIPs(w, report.EIPs) +
		printPVCs(w, report.PVCs)

	fmt.Fprintln(w, "Scan complete.")

	if total == 0 {
		fmt.Fprintln(w, "No orphaned resources detected. Your infrastructure looks clean!")
		return
	}

	fmt.Fprintln(w, "=========================================")
	fmt.Fprintf(w, "Total Monthly Waste Detected: $%.2f\n", total)
	fmt.Fprintln(w, "=========================================")
	fmt.Fprintln(w, "> Automated cleanup (OrphanSweep) coming soon.")
	fmt.Fprintln(w, "> More from ClusterLedger: https://github.com/ClusterLedger")
}

func printVolumes(w io.Writer, volumes []scanner.OrphanedVolume) float64 {
	if len(volumes) == 0 {
		return 0
	}

	byRegion := groupBy(volumes, func(v scanner.OrphanedVolume) string { return v.Region })
	var total float64

	for region, vols := range byRegion {
		cost := sumCost(vols, func(v scanner.OrphanedVolume) float64 {
			return pricing.EBSMonthlyCost(v.SizeGB)
		})
		total += cost
		writeFinding(w, len(vols), "Unattached EBS Volumes", region, cost)
	}

	return total
}

func printEIPs(w io.Writer, eips []scanner.OrphanedEIP) float64 {
	if len(eips) == 0 {
		return 0
	}

	cost := pricing.EIPMonthlyCost(len(eips))
	fmt.Fprintf(w, "[Found] %d Unassociated Elastic IPs. Wasting: $%.2f/mo\n", len(eips), cost)

	return cost
}

func printLBs(w io.Writer, lbs []scanner.IdleLoadBalancer) float64 {
	if len(lbs) == 0 {
		return 0
	}

	byRegion := groupBy(lbs, func(lb scanner.IdleLoadBalancer) string { return lb.Region })
	var total float64

	for region, regionLBs := range byRegion {
		cost := sumCost(regionLBs, func(lb scanner.IdleLoadBalancer) float64 {
			return pricing.LBMonthlyCost(lb.Type)
		})
		total += cost
		writeFinding(w, len(regionLBs), "Idle Load Balancers", region, cost)
	}

	return total
}

func printPVCs(w io.Writer, pvcs []scanner.OrphanedPVC) float64 {
	if len(pvcs) == 0 {
		return 0
	}

	namespaces := uniqueNamespaces(pvcs)
	cost := sumCost(pvcs, func(pvc scanner.OrphanedPVC) float64 {
		return pricing.PVCMonthlyCost(pvc.SizeGB)
	})
	writeFinding(w, len(pvcs), "Unattached K8s PVCs", strings.Join(namespaces, ", "), cost)
	return cost
}

// --- helpers ---

// writeFinding formats a single "[Found]" line with a regional/namespace qualifier.
func writeFinding(w io.Writer, count int, label, qualifier string, cost float64) {
	fmt.Fprintf(w, "[Found] %d %s (%s). Wasting: $%.2f/mo\n", count, label, qualifier, cost)
}

// groupBy partitions items into buckets keyed by the given function.
func groupBy[T any](items []T, key func(T) string) map[string][]T {
	m := make(map[string][]T, len(items))
	for _, item := range items {
		k := key(item)
		m[k] = append(m[k], item)
	}
	return m
}

// sumCost returns the total cost across items using the given per-item cost function.
func sumCost[T any](items []T, cost func(T) float64) float64 {
	var total float64
	for _, item := range items {
		total += cost(item)
	}
	return total
}

func uniqueNamespaces(pvcs []scanner.OrphanedPVC) []string {
	seen := make(map[string]bool, len(pvcs))
	var result []string
	for _, pvc := range pvcs {
		if !seen[pvc.Namespace] {
			seen[pvc.Namespace] = true
			result = append(result, pvc.Namespace)
		}
	}
	return result
}
