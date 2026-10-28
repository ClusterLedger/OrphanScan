package scanner

import "github.com/ClusterLedger/OrphanScan/pkg/diagnostic"

const (
	SourceAWS        = "aws"
	SourceKubernetes = "kubernetes"
	StatusComplete   = "complete"
	StatusPartial    = "partial"
	StatusFailed     = "failed"
)

// Check records whether a requested resource check actually completed. Errors
// are sanitized before they enter a report or a warning callback.
type Check struct {
	Source   string `json:"source"`
	Region   string `json:"region,omitempty"`
	Resource string `json:"resource"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

func newCheck(resource, region string, err error) Check {
	check := Check{Source: SourceAWS, Region: region, Resource: resource, Status: StatusComplete}
	if err != nil {
		check.Status = StatusFailed
		check.Error = diagnostic.Message(err)
	}
	return check
}

// OverallStatus never describes an unattempted or wholly failed scan as clean.
func OverallStatus(checks []Check) string {
	usable, incomplete := false, false
	for _, check := range checks {
		usable = usable || check.Status == StatusComplete || check.Status == StatusPartial
		incomplete = incomplete || check.Status != StatusComplete
	}
	if !usable {
		return StatusFailed
	}
	if incomplete {
		return StatusPartial
	}
	return StatusComplete
}
