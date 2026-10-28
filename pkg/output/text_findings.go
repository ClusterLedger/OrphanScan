package output

import (
	"cmp"
	"slices"

	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type findingRow struct {
	source, region, resource string
	count                    int
	cost                     float64
}

func summarizeFindings(report WasteReport) []findingRow {
	rows := regionalFindings(report.Volumes, "ebs_volumes", func(v scanner.OrphanedVolume) string { return v.Region }, func(v scanner.OrphanedVolume) float64 { return pricing.EBSMonthlyCost(v.SizeGB) })
	rows = append(rows, regionalFindings(report.EIPs, "elastic_ips", func(v scanner.OrphanedEIP) string { return v.Region }, func(scanner.OrphanedEIP) float64 { return pricing.EIPMonthlyCost(1) })...)
	rows = append(rows, regionalFindings(report.LBs, "load_balancers", func(v scanner.IdleLoadBalancer) string { return v.Region }, pricing.LoadBalancerMonthlyCost)...)
	if len(report.PVCs) > 0 {
		cost := sumCost(report.PVCs, func(v scanner.OrphanedPVC) float64 { return pricing.PVCMonthlyCost(v.SizeGB) })
		rows = append(rows, findingRow{source: scanner.SourceKubernetes, resource: "persistent_volume_claims", count: len(report.PVCs), cost: pricing.RoundUSD(cost)})
	}
	slices.SortFunc(rows, func(a, b findingRow) int {
		return cmp.Or(cmp.Compare(a.source, b.source), cmp.Compare(a.region, b.region), cmp.Compare(a.resource, b.resource))
	})
	return rows
}

func regionalFindings[T any](items []T, resource string, region func(T) string, cost func(T) float64) []findingRow {
	groups := groupBy(items, region)
	rows := make([]findingRow, 0, len(groups))
	for _, name := range sortedKeys(groups) {
		rows = append(rows, findingRow{source: scanner.SourceAWS, region: name, resource: resource, count: len(groups[name]), cost: pricing.RoundUSD(sumCost(groups[name], cost))})
	}
	return rows
}

func printFindings(w *textWriter, report WasteReport, sources []sourceSummary) {
	rows := summarizeFindings(report)
	if len(rows) > 0 {
		w.println("Findings (candidates for review)")
		previous := ""
		for _, row := range rows {
			scope := sourceLabel(row.source)
			if row.source == scanner.SourceAWS {
				scope += " / " + regionLabel(row.region)
			}
			if scope != previous {
				w.printf("  %s\n", scope)
				previous = scope
			}
			w.printf("    %-34s %3d   $%.2f/month\n", resourceLabel(row.resource), row.count, row.cost)
		}
		w.println()
	} else if textStatus(sources) == "Partial" {
		w.println("No priced candidates found in available results.")
		w.println()
	} else if textStatus(sources) == "Complete" {
		w.println("No priced candidates found by the completed checks.")
		w.println()
	}
	printReviews(w, report.LBReviews)
}

func printReviews(w *textWriter, reviews []scanner.LoadBalancerReview) {
	if len(reviews) == 0 {
		return
	}
	w.println("Unpriced reviews")
	groups := groupBy(reviews, func(v scanner.LoadBalancerReview) string { return v.Region })
	for _, region := range sortedKeys(groups) {
		w.printf("  AWS / %s: %s flagged for investigation.\n", regionLabel(region), quantified(len(groups[region]), "load balancer"))
	}
	w.println("  Excluded from baseline estimates.")
	w.println()
}
