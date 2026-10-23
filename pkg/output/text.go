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

// PrintText writes a human-readable waste audit to w and returns any write error.
func PrintText(w io.Writer, report WasteReport) error {
	tw := &textWriter{w: w}
	tw.println("=== OrphanScan: Waste Audit ===")
	tw.println("Scanning local kubeconfig and AWS environment...")
	tw.println()

	total := printVolumes(tw, report.Volumes) +
		printLBs(tw, report.LBs) +
		printEIPs(tw, report.EIPs) +
		printPVCs(tw, report.PVCs)

	tw.println("Scan complete.")

	if total == 0 {
		tw.println("No orphaned resources detected. Your infrastructure looks clean!")
		return tw.err
	}

	tw.println("=========================================")
	tw.printf("Total Monthly Waste Detected: $%.2f\n", total)
	tw.println("=========================================")
	tw.println("> Automated cleanup (OrphanSweep) coming soon.")
	tw.println("> More from ClusterLedger: https://github.com/ClusterLedger")
	return tw.err
}

// textWriter retains the first write error and skips any subsequent writes.
type textWriter struct {
	w   io.Writer
	err error
}

func (tw *textWriter) println(args ...any) {
	if tw.err != nil {
		return
	}
	_, tw.err = fmt.Fprintln(tw.w, args...)
}

func (tw *textWriter) printf(format string, args ...any) {
	if tw.err != nil {
		return
	}
	_, tw.err = fmt.Fprintf(tw.w, format, args...)
}

func printVolumes(w *textWriter, volumes []scanner.OrphanedVolume) float64 {
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

func printEIPs(w *textWriter, eips []scanner.OrphanedEIP) float64 {
	if len(eips) == 0 {
		return 0
	}

	cost := pricing.EIPMonthlyCost(len(eips))
	w.printf("[Found] %d Unassociated Elastic IPs. Wasting: $%.2f/mo\n", len(eips), cost)

	return cost
}

func printLBs(w *textWriter, lbs []scanner.IdleLoadBalancer) float64 {
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

func printPVCs(w *textWriter, pvcs []scanner.OrphanedPVC) float64 {
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
func writeFinding(w *textWriter, count int, label, qualifier string, cost float64) {
	w.printf("[Found] %d %s (%s). Wasting: $%.2f/mo\n", count, label, qualifier, cost)
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
