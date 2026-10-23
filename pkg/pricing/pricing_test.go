package pricing

import (
	"math"
	"testing"
)

func TestEBSMonthlyCost(t *testing.T) {
	tests := []struct {
		name   string
		sizeGB int32
		want   float64
	}{
		{"100GB volume", 100, 8.00},
		{"500GB volume", 500, 40.00},
		{"zero size", 0, 0.00},
		{"1GB volume", 1, 0.08},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EBSMonthlyCost(tt.sizeGB)
			if !almostEqual(got, tt.want) {
				t.Errorf("EBSMonthlyCost(%d) = %.2f, want %.2f", tt.sizeGB, got, tt.want)
			}
		})
	}
}

func TestPVCMonthlyCost(t *testing.T) {
	got := PVCMonthlyCost(10.0)
	want := 0.80
	if !almostEqual(got, want) {
		t.Errorf("PVCMonthlyCost(10.0) = %.2f, want %.2f", got, want)
	}
}

func TestEIPMonthlyCost(t *testing.T) {
	got := EIPMonthlyCost(5)
	want := 18.00
	if !almostEqual(got, want) {
		t.Errorf("EIPMonthlyCost(5) = %.2f, want %.2f", got, want)
	}
}

func TestLBMonthlyCost(t *testing.T) {
	tests := []struct {
		lbType string
		want   float64
	}{
		{"CLB", 18.00},
		{"ALB", 22.00},
		{"NLB", 16.00},
		{"unknown", 22.00}, // defaults to ALB rate
	}

	for _, tt := range tests {
		t.Run(tt.lbType, func(t *testing.T) {
			got := LBMonthlyCost(tt.lbType)
			if !almostEqual(got, tt.want) {
				t.Errorf("LBMonthlyCost(%s) = %.2f, want %.2f", tt.lbType, got, tt.want)
			}
		})
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}
