package output

import (
	"fmt"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type sourceSummary struct {
	source              string
	checks              []scanner.Check
	complete, partial   int
	resourceChecks      int
	regions             map[string]bool
	candidates, reviews int
}

func summarizeSources(report WasteReport) []sourceSummary {
	bySource := make(map[string][]scanner.Check)
	if report.HasAWS || len(report.Volumes)+len(report.EIPs)+len(report.LBs)+len(report.LBReviews) > 0 {
		bySource[scanner.SourceAWS] = nil
	}
	if report.HasK8s || len(report.PVCs) > 0 {
		bySource[scanner.SourceKubernetes] = nil
	}
	for _, check := range report.Checks {
		bySource[check.Source] = append(bySource[check.Source], check)
	}
	sources := make([]sourceSummary, 0, len(bySource))
	for _, source := range sortedKeys(bySource) {
		summary := sourceSummary{source: source, checks: bySource[source], regions: make(map[string]bool)}
		for _, check := range summary.checks {
			if check.Resource == "configuration" {
				continue
			}
			summary.resourceChecks++
			switch check.Status {
			case scanner.StatusComplete:
				summary.complete++
			case scanner.StatusPartial:
				summary.partial++
			}
			if scanner.IsValidRegionName(check.Region) {
				summary.regions[check.Region] = true
			}
		}
		switch source {
		case scanner.SourceAWS:
			summary.candidates = len(report.Volumes) + len(report.EIPs) + len(report.LBs)
			summary.reviews = len(report.LBReviews)
		case scanner.SourceKubernetes:
			summary.candidates = len(report.PVCs)
		}
		sources = append(sources, summary)
	}
	return sources
}

func (s sourceSummary) state() string {
	switch {
	case len(s.checks) == 0:
		return "Unknown"
	case s.resourceChecks == 0:
		return "Not scanned"
	case s.complete+s.partial == 0:
		return "Failed"
	case s.complete != len(s.checks):
		return "Partial"
	default:
		return "Complete"
	}
}

func (s sourceSummary) description() string {
	switch s.state() {
	case "Unknown":
		return "Unknown - check coverage was not recorded"
	case "Complete":
		coverage := quantified(s.complete, "check")
		if len(s.regions) > 0 {
			coverage += " across " + quantified(len(s.regions), "region")
		}
		if s.candidates+s.reviews == 0 {
			if s.source == scanner.SourceKubernetes {
				return "Complete - no PVC candidates found"
			}
			return "Complete - no candidates found (" + coverage + ")"
		}
		return "Complete - " + coverage
	case "Partial":
		return fmt.Sprintf("Partial - %d complete, %d partial, %d incomplete checks", s.complete, s.partial, len(s.checks)-s.complete-s.partial)
	case "Failed":
		return "Failed - no usable resource results"
	default:
		return "Not scanned"
	}
}

func (s sourceSummary) estimate(cost float64) string {
	amount := fmt.Sprintf("$%.2f/month", cost)
	switch s.state() {
	case "Complete":
		return amount
	case "Partial":
		return amount + " (available results only)"
	case "Unknown":
		if s.candidates > 0 {
			return amount + " (coverage unknown)"
		}
	}
	return "Unavailable"
}

func textStatus(sources []sourceSummary) string {
	usable, incomplete := false, false
	for _, source := range sources {
		state := source.state()
		usable = usable || state == "Complete" || state == "Partial"
		incomplete = incomplete || state != "Complete"
	}
	if !usable {
		for _, source := range sources {
			if len(source.checks) > 0 {
				return "Failed"
			}
		}
		return "Unknown"
	}
	if incomplete {
		return "Partial"
	}
	return "Complete"
}

func printSources(w *textWriter, sources []sourceSummary) {
	w.println("Sources")
	if len(sources) == 0 {
		w.println("  No source results were recorded.")
	}
	for _, source := range sources {
		w.printf("  %-12s %s\n", sourceLabel(source.source), source.description())
	}
	w.println()
}

func printEstimates(w *textWriter, report WasteReport, sources []sourceSummary) {
	w.println("Monthly baseline estimates")
	costs := report.Costs()
	if len(sources) == 0 {
		w.println("  Unavailable - no resource results were recorded.")
	}
	for _, source := range sources {
		estimate := "Unavailable"
		switch source.source {
		case scanner.SourceAWS:
			estimate = source.estimate(costs.AWS)
		case scanner.SourceKubernetes:
			estimate = source.estimate(costs.Kubernetes)
		}
		w.printf("  %-12s %s\n", sourceLabel(source.source), estimate)
	}
	w.println()
}

func printChecks(w *textWriter, checks []scanner.Check) {
	w.println("Checks")
	if len(checks) == 0 {
		w.println("  Check coverage was not recorded.")
	}
	for _, check := range checks {
		scope := sourceLabel(check.Source)
		if check.Region != "" {
			scope += " / " + regionLabel(check.Region)
		}
		w.printf("  %-24s %-30s %s\n", scope, resourceLabel(check.Resource), checkStatusLabel(check.Status))
	}
	w.println()
}

func sourceLabel(source string) string {
	switch source {
	case scanner.SourceAWS:
		return "AWS"
	case scanner.SourceKubernetes:
		return "Kubernetes"
	default:
		return "Other source"
	}
}

func regionLabel(region string) string {
	if scanner.IsValidRegionName(region) {
		return region
	}
	return "Unknown region"
}

func resourceLabel(resource string) string {
	switch resource {
	case "ebs_volumes":
		return "Unattached EBS volumes"
	case "elastic_ips":
		return "Unassociated Elastic IPs"
	case "classic_load_balancers":
		return "Classic load balancers"
	case "elbv2_load_balancers":
		return "ALB / NLB / GWLB"
	case "load_balancers":
		return "Load balancers without backends"
	case "persistent_volume_claims":
		return "PVC candidates"
	case "configuration":
		return "Configuration / region discovery"
	default:
		return "Resource check"
	}
}

func checkStatusLabel(status string) string {
	switch status {
	case scanner.StatusComplete:
		return "Complete"
	case scanner.StatusPartial:
		return "Partial"
	case scanner.StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

func quantified(count int, noun string) string {
	if count != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", count, noun)
}
