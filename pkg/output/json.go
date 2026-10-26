package output

import (
	"encoding/json"
	"io"

	"github.com/ClusterLedger/OrphanScan/pkg/pricing"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// --- JSON wire types ---

type jsonReport struct {
	Volumes []jsonVolume `json:"unattached_ebs_volumes,omitempty"`
	EIPs    []jsonEIP    `json:"unassociated_eips,omitempty"`
	LBs     []jsonLB     `json:"idle_load_balancers,omitempty"`
	PVCs    []jsonPVC    `json:"unattached_pvcs,omitempty"`
	Total   float64      `json:"total_monthly_waste"`
}

type jsonVolume struct {
	VolumeID    string  `json:"volume_id"`
	Region      string  `json:"region"`
	SizeGB      int32   `json:"size_gb"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type jsonEIP struct {
	AllocationID string  `json:"allocation_id"`
	Region       string  `json:"region"`
	MonthlyCost  float64 `json:"monthly_cost"`
}

type jsonLB struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Region      string  `json:"region"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type jsonPVC struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	SizeGB      float64 `json:"size_gb"`
	MonthlyCost float64 `json:"monthly_cost"`
}

// --- public entry point ---

// PrintJSON encodes the waste report as pretty-printed JSON.
func PrintJSON(w io.Writer, report WasteReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildJSONReport(report))
}

// --- report assembly ---

func buildJSONReport(report WasteReport) jsonReport {
	volumes, volCost := mapVolumes(report.Volumes)
	eips, eipCost := mapEIPs(report.EIPs)
	lbs, lbCost := mapLBs(report.LBs)
	pvcs, pvcCost := mapPVCs(report.PVCs)

	return jsonReport{
		Volumes: volumes,
		EIPs:    eips,
		LBs:     lbs,
		PVCs:    pvcs,
		Total:   volCost + eipCost + lbCost + pvcCost,
	}
}

// --- per-resource mappers ---

func mapVolumes(volumes []scanner.OrphanedVolume) ([]jsonVolume, float64) {
	out := make([]jsonVolume, 0, len(volumes))
	var total float64

	for _, v := range volumes {
		cost := pricing.EBSMonthlyCost(v.SizeGB)
		total += cost
		out = append(out, jsonVolume{
			VolumeID:    obfuscate(v.VolumeID),
			Region:      v.Region,
			SizeGB:      v.SizeGB,
			MonthlyCost: cost,
		})
	}

	return out, total
}

func mapEIPs(eips []scanner.OrphanedEIP) ([]jsonEIP, float64) {
	out := make([]jsonEIP, 0, len(eips))
	var total float64

	for _, e := range eips {
		cost := pricing.EIPMonthlyCost(1)
		total += cost
		out = append(out, jsonEIP{
			AllocationID: obfuscate(e.AllocationID),
			Region:       e.Region,
			MonthlyCost:  cost,
		})
	}

	return out, total
}

func mapLBs(lbs []scanner.IdleLoadBalancer) ([]jsonLB, float64) {
	out := make([]jsonLB, 0, len(lbs))
	var total float64

	for _, lb := range lbs {
		cost := pricing.LBMonthlyCost(lb.Type)
		total += cost
		out = append(out, jsonLB{
			Name:        obfuscate(lb.Name),
			Type:        lb.Type,
			Region:      lb.Region,
			MonthlyCost: cost,
		})
	}

	return out, total
}

func mapPVCs(pvcs []scanner.OrphanedPVC) ([]jsonPVC, float64) {
	out := make([]jsonPVC, 0, len(pvcs))
	var total float64

	for _, pvc := range pvcs {
		cost := pricing.PVCMonthlyCost(pvc.SizeGB)
		total += cost
		out = append(out, jsonPVC{
			Name:        obfuscate(pvc.Name),
			Namespace:   pvc.Namespace,
			SizeGB:      pvc.SizeGB,
			MonthlyCost: cost,
		})
	}

	return out, total
}
