package pricing

import "github.com/ClusterLedger/OrphanScan/pkg/scanner"

const (
	RateEBSPerGB = 0.08
	RatePVCPerGB = 0.08
	RateEIP      = 3.60
	RateCLB      = 18.00
	RateALB      = 22.00
	RateNLB      = 16.00
	RateGWLB     = 18.00
)

func EBSMonthlyCost(sizeGB int32) float64 {
	return float64(sizeGB) * RateEBSPerGB
}

func PVCMonthlyCost(sizeGB float64) float64 {
	return sizeGB * RatePVCPerGB
}

func EIPMonthlyCost(count int) float64 {
	return float64(count) * RateEIP
}

func LBMonthlyCost(lbType string) float64 {
	switch lbType {
	case scanner.LBTypeCLB:
		return RateCLB
	case scanner.LBTypeALB:
		return RateALB
	case scanner.LBTypeNLB:
		return RateNLB
	case scanner.LBTypeGWLB:
		return RateGWLB
	default:
		return 0
	}
}
