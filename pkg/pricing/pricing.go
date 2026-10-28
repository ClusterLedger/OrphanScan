// Package pricing provides documented reference estimates, not billing data.
package pricing

import (
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
	"math"
)

// Reference prices verified against AWS's public us-east-1 pricing on this date.
// Hourly rates use a 30-day / 720-hour month and exclude usage and taxes.
const (
	ReferenceRegion = "us-east-1"
	ReferenceDate   = "2026-10-26"
	HoursPerMonth   = 720
	RateEBSPerGB    = 0.08
	RatePVCPerGB    = 0.08
	RateEIP         = 0.005 * HoursPerMonth
	RateCLB         = 0.025 * HoursPerMonth
	RateALB         = 0.0225 * HoursPerMonth
	RateNLB         = 0.0225 * HoursPerMonth
	RateGWLB        = 0.0125 * HoursPerMonth
)

func RoundUSD(value float64) float64        { return math.Round(value*100) / 100 }
func EBSMonthlyCost(sizeGB int32) float64   { return RoundUSD(float64(sizeGB) * RateEBSPerGB) }
func PVCMonthlyCost(sizeGB float64) float64 { return RoundUSD(sizeGB * RatePVCPerGB) }
func EIPMonthlyCost(count int) float64      { return RoundUSD(float64(count) * RateEIP) }
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

func LoadBalancerMonthlyCost(lb scanner.IdleLoadBalancer) float64 {
	rate := LBMonthlyCost(lb.Type)
	if lb.Type == scanner.LBTypeGWLB {
		rate *= float64(max(1, lb.Zones))
	}
	return RoundUSD(rate)
}
