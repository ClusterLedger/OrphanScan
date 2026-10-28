package scanner

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	clbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	v2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type pagedV2 struct {
	stubELBV2Client
	groups  func(*elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error)
	targets func(*elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error)
}

func (c *pagedV2) DescribeTargetGroups(_ context.Context, in *elbv2.DescribeTargetGroupsInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error) {
	return c.groups(in)
}
func (c *pagedV2) DescribeTargetHealth(_ context.Context, in *elbv2.DescribeTargetHealthInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error) {
	return c.targets(in)
}

func TestOnlyEmptyBackendsCountAsWaste(t *testing.T) {
	for _, tt := range []struct {
		name         string
		targets      []v2types.TargetHealthDescription
		disabled     bool
		idle, review int
	}{
		{name: "empty", idle: 1},
		{name: "healthy", targets: []v2types.TargetHealthDescription{{TargetHealth: &v2types.TargetHealth{State: v2types.TargetHealthStateEnumHealthy}}}},
		{name: "unhealthy registered", targets: []v2types.TargetHealthDescription{{TargetHealth: &v2types.TargetHealth{State: v2types.TargetHealthStateEnumUnhealthy}}}, review: 1},
		{name: "Lambda health checks disabled", targets: []v2types.TargetHealthDescription{{TargetHealth: &v2types.TargetHealth{State: v2types.TargetHealthStateEnumUnavailable}}}, disabled: true, review: 1},
		{name: "registered health missing", targets: []v2types.TargetHealthDescription{{}}, review: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubELBV2Client{
				lbs:    &elbv2.DescribeLoadBalancersOutput{LoadBalancers: []v2types.LoadBalancer{{LoadBalancerArn: aws.String("arn:lb"), LoadBalancerName: aws.String("private-alb-name"), Type: v2types.LoadBalancerTypeEnumApplication}}},
				tgs:    &elbv2.DescribeTargetGroupsOutput{TargetGroups: []v2types.TargetGroup{{TargetGroupArn: aws.String("arn:tg"), HealthCheckEnabled: aws.Bool(!tt.disabled)}}},
				health: &elbv2.DescribeTargetHealthOutput{TargetHealthDescriptions: tt.targets},
			}
			result, err := scanELBv2(context.Background(), client, "us-east-1", nil)
			if err != nil || len(result.Idle) != tt.idle || len(result.Review) != tt.review {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if tt.disabled && result.Review[0].Reason != "health_checks_disabled" {
				t.Fatal("disabled checks lost their reason")
			}
		})
	}
	classic := &stubELBClient{lbs: &elb.DescribeLoadBalancersOutput{LoadBalancerDescriptions: []clbtypes.LoadBalancerDescription{{LoadBalancerName: aws.String("empty")}, {LoadBalancerName: aws.String("unhealthy"), Instances: []clbtypes.Instance{{InstanceId: aws.String("i-private")}}}}}, health: &elb.DescribeInstanceHealthOutput{InstanceStates: []clbtypes.InstanceState{{State: aws.String("OutOfService")}}}}
	result, err := scanClassicLBs(context.Background(), classic, "us-east-1", nil)
	if err != nil || len(result.Idle) != 1 || result.Idle[0].Name != "empty" || len(result.Review) != 1 {
		t.Fatalf("CLB result=%+v err=%v", result, err)
	}
}

func TestTargetGroupsIncludeLaterPages(t *testing.T) {
	var pages []string
	client := &pagedV2{
		stubELBV2Client: stubELBV2Client{lbs: &elbv2.DescribeLoadBalancersOutput{LoadBalancers: []v2types.LoadBalancer{{LoadBalancerArn: aws.String("arn:lb"), LoadBalancerName: aws.String("active-alb"), Type: v2types.LoadBalancerTypeEnumApplication}}}},
		groups: func(in *elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error) {
			pages = append(pages, safeString(in.Marker))
			if in.Marker == nil {
				return &elbv2.DescribeTargetGroupsOutput{TargetGroups: []v2types.TargetGroup{{TargetGroupArn: aws.String("unhealthy")}}, NextMarker: aws.String("second")}, nil
			}
			if *in.Marker != "second" {
				t.Errorf("unexpected marker %s", *in.Marker)
			}
			return &elbv2.DescribeTargetGroupsOutput{TargetGroups: []v2types.TargetGroup{{TargetGroupArn: aws.String("healthy")}}}, nil
		},
		targets: func(in *elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error) {
			state := v2types.TargetHealthStateEnumUnhealthy
			if *in.TargetGroupArn == "healthy" {
				state = v2types.TargetHealthStateEnumHealthy
			}
			return &elbv2.DescribeTargetHealthOutput{TargetHealthDescriptions: []v2types.TargetHealthDescription{{TargetHealth: &v2types.TargetHealth{State: state}}}}, nil
		},
	}
	result, err := scanELBv2(context.Background(), client, "us-east-1", nil)
	if err != nil || len(pages) != 2 || len(result.Idle) != 0 || len(result.Review) != 0 {
		t.Fatalf("pages=%v result=%+v err=%v", pages, result, err)
	}
}

func TestFailedTargetGroupPageCannotCreateIdleFinding(t *testing.T) {
	calls := 0
	client := &pagedV2{stubELBV2Client: stubELBV2Client{lbs: &elbv2.DescribeLoadBalancersOutput{LoadBalancers: []v2types.LoadBalancer{{LoadBalancerArn: aws.String("arn:secret"), LoadBalancerName: aws.String("private-alb-name"), Type: v2types.LoadBalancerTypeEnumApplication}}}}, groups: func(*elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error) {
		calls++
		if calls == 1 {
			return &elbv2.DescribeTargetGroupsOutput{NextMarker: aws.String("next")}, nil
		}
		return nil, errors.New("https://private-cluster.invalid/private-alb-name")
	}}
	var warning string
	result, err := scanELBv2(context.Background(), client, "us-east-1", func(format string, args ...any) { warning = format })
	if err != nil || len(result.Idle) != 0 || len(result.Errors) != 1 || strings.Contains(strings.Join(result.Errors, "")+warning, "private") {
		t.Fatalf("unsafe or incorrect result=%+v err=%v", result, err)
	}
}

func completeClients() RegionClients {
	return RegionClients{EC2: &mockEC2Client{addresses: &ec2.DescribeAddressesOutput{}}, ELB: &stubELBClient{lbs: &elb.DescribeLoadBalancersOutput{}}, ELBV2: &stubELBV2Client{lbs: &elbv2.DescribeLoadBalancersOutput{}}}
}

func TestAWSConcurrencyAndDuplicateRegions(t *testing.T) {
	started := make(chan string, 10)
	release := make(chan struct{})
	var active, peak, calls atomic.Int32
	factory := func(_ aws.Config, region string) RegionClients {
		calls.Add(1)
		n := active.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- region
		<-release
		active.Add(-1)
		return completeClients()
	}
	done := make(chan AWSScanResult, 1)
	go func() {
		done <- ScanAWSWithConcurrency(context.Background(), aws.Config{}, []string{"a", "b", "c", "a", "d"}, factory, nil, 2)
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-started:
		close(release)
		t.Fatal("more than two regions started concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case result := <-done:
		if calls.Load() != 4 || peak.Load() > 2 || len(result.Checks) != 16 || OverallStatus(result.Checks) != StatusComplete {
			t.Fatalf("calls=%d peak=%d checks=%v", calls.Load(), peak.Load(), result.Checks)
		}
	case <-time.After(time.Second):
		t.Fatal("scan did not finish")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := ScanAWSWithConcurrency(ctx, aws.Config{}, []string{"a", "b"}, func(aws.Config, string) RegionClients {
		t.Error("cancelled scan initialized clients")
		return completeClients()
	}, nil, 2)
	if len(result.Checks) != 8 || OverallStatus(result.Checks) != StatusFailed {
		t.Fatalf("cancelled checks=%v", result.Checks)
	}
}

func TestEveryPodPhaseProtectsPVC(t *testing.T) {
	for _, phase := range []corev1.PodPhase{corev1.PodPending, corev1.PodRunning, corev1.PodUnknown, corev1.PodSucceeded, corev1.PodFailed} {
		t.Run(string(phase), func(t *testing.T) {
			pod := makePodWithPVC("workload", "default", "data")
			pod.Status.Phase = phase
			client := &mockKubeClient{pvcs: []corev1.PersistentVolumeClaim{makePVC("data", "default", 10, 48*time.Hour, corev1.ClaimBound)}, pods: []corev1.Pod{pod}}
			result, err := ScanOrphanedPVCs(context.Background(), client)
			if err != nil || len(result) != 0 {
				t.Fatalf("phase %s: %v %v", phase, result, err)
			}
		})
	}
}

func TestPodOwnedEphemeralPVCIsProtected(t *testing.T) {
	pvc := makePVC("pod-ephemeral-data", "default", 10, 48*time.Hour, corev1.ClaimBound)
	pvc.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: "workload", UID: "pod-uid"}}
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "default", UID: "pod-uid"}}
	result, err := ScanOrphanedPVCs(context.Background(), &mockKubeClient{pvcs: []corev1.PersistentVolumeClaim{pvc}, pods: []corev1.Pod{pod}})
	if err != nil || len(result) != 0 {
		t.Fatalf("pod-owned PVC reported as orphan: %v %v", result, err)
	}
}
