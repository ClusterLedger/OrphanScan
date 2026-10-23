package scanner

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Mock KubeClient ---

type mockKubeClient struct {
	pvcs []corev1.PersistentVolumeClaim
	pods []corev1.Pod
	err  error
}

func (m *mockKubeClient) ListPVCs(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
	return m.pvcs, m.err
}

func (m *mockKubeClient) ListRunningPods(ctx context.Context) ([]corev1.Pod, error) {
	return m.pods, m.err
}

// --- Helper to build test PVCs ---

func makePVC(name, namespace string, sizeGi int, age time.Duration, phase corev1.PersistentVolumeClaimPhase) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", sizeGi)),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}

// --- Helper to build test Pods that mount a PVC ---

func makePodWithPVC(name, namespace, pvcName string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
}

// --- Tests ---

func TestScanOrphanedPVCs_FindsOrphans(t *testing.T) {
	client := &mockKubeClient{
		pvcs: []corev1.PersistentVolumeClaim{
			makePVC("orphan-pvc", "default", 10, 48*time.Hour, corev1.ClaimBound),
			makePVC("used-pvc", "default", 20, 48*time.Hour, corev1.ClaimBound),
		},
		pods: []corev1.Pod{
			makePodWithPVC("my-app", "default", "used-pvc"),
		},
	}

	result, err := ScanOrphanedPVCs(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(result))
	}

	if result[0].Name != "orphan-pvc" {
		t.Errorf("expected orphan-pvc, got %s", result[0].Name)
	}
}

func TestScanOrphanedPVCs_24hSafetyFilter(t *testing.T) {
	client := &mockKubeClient{
		pvcs: []corev1.PersistentVolumeClaim{
			makePVC("new-pvc", "default", 10, 2*time.Hour, corev1.ClaimBound), // only 2h old
		},
		pods: []corev1.Pod{},
	}

	result, err := ScanOrphanedPVCs(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 orphans (24h filter), got %d", len(result))
	}
}

func TestScanOrphanedPVCs_SkipsPending(t *testing.T) {
	client := &mockKubeClient{
		pvcs: []corev1.PersistentVolumeClaim{
			makePVC("pending-pvc", "default", 10, 48*time.Hour, corev1.ClaimPending), // Pending, no real disk
		},
		pods: []corev1.Pod{},
	}

	result, err := ScanOrphanedPVCs(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 orphans (pending PVC), got %d", len(result))
	}
}

func TestScanOrphanedPVCs_NoneOrphaned(t *testing.T) {
	client := &mockKubeClient{
		pvcs: []corev1.PersistentVolumeClaim{
			makePVC("attached-pvc", "staging", 5, 72*time.Hour, corev1.ClaimBound),
		},
		pods: []corev1.Pod{
			makePodWithPVC("worker", "staging", "attached-pvc"),
		},
	}

	result, err := ScanOrphanedPVCs(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(result))
	}
}
