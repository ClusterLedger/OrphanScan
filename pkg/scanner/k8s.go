package scanner

import (
	"context"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type KubeClient interface {
	ListPVCs(ctx context.Context) ([]corev1.PersistentVolumeClaim, error)
	ListPods(ctx context.Context) ([]corev1.Pod, error)
}

type OrphanedPVC struct {
	Name      string
	Namespace string
	SizeGB    float64
	Age       time.Duration
}

const orphanAgeThreshold = 24 * time.Hour

func ScanOrphanedPVCs(ctx context.Context, client KubeClient) ([]OrphanedPVC, error) {
	pvcs, err := client.ListPVCs(ctx)
	if err != nil {
		return nil, err
	}

	pods, err := client.ListPods(ctx)
	if err != nil {
		return nil, err
	}

	usedPVCs := getUsedPVCs(pods)

	return filterOrphanedPVCs(pvcs, usedPVCs, time.Now()), nil
}

func filterOrphanedPVCs(pvcs []corev1.PersistentVolumeClaim, usedPVCs map[string]bool, now time.Time) []OrphanedPVC {
	var orphaned []OrphanedPVC

	for _, pvc := range pvcs {
		if usedPVCs[pvcKey(pvc)] {
			continue
		}
		// Generic ephemeral PVCs are owned by a Pod rather than named in a
		// PersistentVolumeClaim volume source. Protect those references too.
		ownedByExistingPod := false
		for _, owner := range pvc.OwnerReferences {
			if owner.Kind == "Pod" && owner.UID != "" && usedPVCs["pod:"+pvc.Namespace+"/"+string(owner.UID)] {
				ownedByExistingPod = true
				break
			}
		}
		if ownedByExistingPod {
			continue
		}

		age := now.Sub(pvc.CreationTimestamp.Time)
		if age < orphanAgeThreshold {
			continue
		}

		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}

		orphaned = append(orphaned, toOrphanedPVC(pvc, age))
	}
	return orphaned
}

func pvcKey(pvc corev1.PersistentVolumeClaim) string {
	return pvc.Namespace + "/" + pvc.Name
}

func toOrphanedPVC(pvc corev1.PersistentVolumeClaim, age time.Duration) OrphanedPVC {
	return OrphanedPVC{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		SizeGB:    quantityToGB(pvc.Spec.Resources.Requests[corev1.ResourceStorage]),
		Age:       age,
	}
}

func getUsedPVCs(pods []corev1.Pod) map[string]bool {
	used := make(map[string]bool)
	for _, pod := range pods {
		if pod.UID != "" {
			used["pod:"+pod.Namespace+"/"+string(pod.UID)] = true
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				used[pod.Namespace+"/"+volume.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	return used
}

func quantityToGB(q resource.Quantity) float64 {
	bytes := q.Value()
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return math.Round(gb*100) / 100
}
