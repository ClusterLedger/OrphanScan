package output

import (
	"fmt"
	"io"
	"slices"

	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

const cleanupNoticeThresholdUSD = 49

// TextOptions controls presentation without changing the report or its costs.
type TextOptions struct {
	Verbose bool
	// Interrupted marks a canceled command while retaining completed results.
	Interrupted bool
}

// PrintText writes the compact report and retains the original formatter API.
func PrintText(w io.Writer, report WasteReport) error {
	return PrintTextWithOptions(w, report, TextOptions{})
}

// PrintTextWithOptions adds optional check coverage and reference pricing.
func PrintTextWithOptions(w io.Writer, report WasteReport, options TextOptions) error {
	report = sortedReport(report)
	sources := summarizeSources(report)
	tw := &textWriter{w: w}
	tw.println("OrphanScan")
	status := textStatus(sources)
	if options.Interrupted {
		status = "Interrupted"
	}
	tw.printf("Status: %s\n", status)
	tw.println()
	printSources(tw, sources)
	printFindings(tw, report, sources)
	printEstimates(tw, report, sources)
	printIssues(tw, report.Checks)
	if options.Verbose {
		printChecks(tw, report.Checks)
		tw.println("Reference pricing")
		tw.printf("  %s; %d hours/month; prices checked %s.\n\n", pricing.ReferenceRegion, pricing.HoursPerMonth, pricing.ReferenceDate)
	}
	if len(sources) > 1 {
		tw.println("Estimates are separate; storage may overlap.")
	}
	tw.println("Baseline estimates. Review candidates before cleanup.")
	if !options.Verbose {
		tw.println("Use --verbose for check and pricing details.")
	}
	if shouldShowCleanupNotice(report) {
		tw.println("> Automated cleanup (OrphanSweep) coming soon.")
		tw.println("> More from ClusterLedger: https://github.com/ClusterLedger")
	}
	return tw.err
}

func shouldShowCleanupNotice(report WasteReport) bool {
	return report.Status() != scanner.StatusFailed && report.PreferredMonthlyEstimate() > cleanupNoticeThresholdUSD
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

// --- helpers ---

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

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
