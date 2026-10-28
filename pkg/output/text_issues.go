package output

import (
	"cmp"
	"io"
	"slices"
	"strings"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type checkIssue struct {
	source, message string
	count           int
	regions         map[string]bool
}

// PrintIssues writes grouped, sanitized check diagnostics for JSON's stderr.
// As with Check.Error, supplied messages must already be sanitized by the caller.
func PrintIssues(w io.Writer, checks []scanner.Check) error {
	tw := &textWriter{w: w}
	printIssues(tw, checks)
	return tw.err
}

func groupIssues(checks []scanner.Check) []checkIssue {
	type key struct{ source, message string }
	groups := make(map[key]*checkIssue)
	for _, check := range checks {
		message := strings.TrimSpace(check.Error)
		if message == "" {
			if check.Status == scanner.StatusComplete {
				continue
			}
			message = "This check did not complete.\nReview check coverage with --verbose and retry."
		}
		k := key{check.Source, message}
		issue := groups[k]
		if issue == nil {
			issue = &checkIssue{source: check.Source, message: message, regions: make(map[string]bool)}
			groups[k] = issue
		}
		issue.count++
		if scanner.IsValidRegionName(check.Region) {
			issue.regions[check.Region] = true
		}
	}
	issues := make([]checkIssue, 0, len(groups))
	for _, issue := range groups {
		issues = append(issues, *issue)
	}
	slices.SortFunc(issues, func(a, b checkIssue) int {
		return cmp.Or(cmp.Compare(a.source, b.source), cmp.Compare(a.message, b.message))
	})
	return issues
}

func printIssues(w *textWriter, checks []scanner.Check) {
	issues := groupIssues(checks)
	if len(issues) == 0 {
		return
	}
	w.println("Action needed")
	for _, issue := range issues {
		scope := sourceLabel(issue.source)
		if len(issue.regions) == 1 {
			scope += " / " + sortedKeys(issue.regions)[0]
		}
		lines := strings.Split(issue.message, "\n")
		w.printf("  %s: %s\n", scope, lines[0])
		for _, line := range lines[1:] {
			w.printf("    %s\n", line)
		}
		if issue.count > 1 {
			w.printf("    Affects %s", quantified(issue.count, "check"))
			if len(issue.regions) > 0 {
				w.printf(" across %s", quantified(len(issue.regions), "region"))
			}
			w.println(".")
		}
	}
	w.println()
}
