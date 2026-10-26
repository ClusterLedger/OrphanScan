package scanner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type stubELBClient struct {
	lbs    *elb.DescribeLoadBalancersOutput
	lbErr  error
	health *elb.DescribeInstanceHealthOutput
	hErr   error
}

func (s *stubELBClient) DescribeLoadBalancers(_ context.Context, _ *elb.DescribeLoadBalancersInput, _ ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error) {
	return s.lbs, s.lbErr
}

func (s *stubELBClient) DescribeInstanceHealth(_ context.Context, _ *elb.DescribeInstanceHealthInput, _ ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error) {
	return s.health, s.hErr
}

type stubELBV2Client struct {
	lbs    *elbv2.DescribeLoadBalancersOutput
	lbErr  error
	tgs    *elbv2.DescribeTargetGroupsOutput
	tgErr  error
	health *elbv2.DescribeTargetHealthOutput
	hErr   error
}

func (s *stubELBV2Client) DescribeLoadBalancers(_ context.Context, _ *elbv2.DescribeLoadBalancersInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	return s.lbs, s.lbErr
}

func (s *stubELBV2Client) DescribeTargetGroups(_ context.Context, _ *elbv2.DescribeTargetGroupsInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error) {
	return s.tgs, s.tgErr
}

func (s *stubELBV2Client) DescribeTargetHealth(_ context.Context, _ *elbv2.DescribeTargetHealthInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error) {
	return s.health, s.hErr
}

func TestScanAWSPartialResults(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, format)
	}

	factory := func(_ aws.Config, _ string) RegionClients {
		return RegionClients{
			EC2: &mockEC2Client{
				volumes: []ec2.DescribeVolumesOutput{{
					Volumes: []types.Volume{
						{VolumeId: aws.String("vol-1"), Size: aws.Int32(50), CreateTime: &old},
					},
				}},
				addrErr: errors.New("eip access denied"),
			},
			ELB: &stubELBClient{
				lbs: &elb.DescribeLoadBalancersOutput{},
			},
			ELBV2: &stubELBV2Client{
				lbs: &elbv2.DescribeLoadBalancersOutput{},
			},
		}
	}

	result := ScanAWS(context.Background(), aws.Config{}, []string{"us-east-1"}, factory, warn)
	if len(result.Volumes) != 1 {
		t.Errorf("expected 1 volume from partial scan, got %d", len(result.Volumes))
	}
	if len(warnings) == 0 {
		t.Error("expected a warning for EIP failure")
	}
}

func TestScanAWSMultiRegion(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	factory := func(_ aws.Config, region string) RegionClients {
		return RegionClients{
			EC2: &mockEC2Client{
				volumes: []ec2.DescribeVolumesOutput{{
					Volumes: []types.Volume{
						{VolumeId: aws.String("vol-" + region), Size: aws.Int32(10), CreateTime: &old},
					},
				}},
				addresses: &ec2.DescribeAddressesOutput{},
			},
			ELB:   &stubELBClient{lbs: &elb.DescribeLoadBalancersOutput{}},
			ELBV2: &stubELBV2Client{lbs: &elbv2.DescribeLoadBalancersOutput{}},
		}
	}

	result := ScanAWS(context.Background(), aws.Config{}, []string{"us-east-1", "eu-west-1"}, factory, nil)
	if len(result.Volumes) != 2 {
		t.Fatalf("expected 2 volumes from 2 regions, got %d", len(result.Volumes))
	}
}

func TestScanIdleClassicLBs_WarnsOnHealthCheckError(t *testing.T) {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, format)
	}
	client := &stubELBClient{
		lbs: &elb.DescribeLoadBalancersOutput{
			LoadBalancerDescriptions: []elbtypes.LoadBalancerDescription{
				{LoadBalancerName: aws.String("my-clb")},
			},
		},
		hErr: errors.New("health check failed"),
	}

	result, err := ScanIdleClassicLBs(context.Background(), client, "us-east-1", warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Error("skipped LB should not appear in results")
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "CLB") {
		t.Errorf("expected CLB health check warning, got %v", warnings)
	}
}

func TestScanIdleELBv2_WarnsOnHealthCheckError(t *testing.T) {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, format)
	}
	client := &stubELBV2Client{
		lbs: &elbv2.DescribeLoadBalancersOutput{
			LoadBalancers: []elbv2types.LoadBalancer{
				{
					LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/my-alb/abc"),
					LoadBalancerName: aws.String("my-alb"),
					Type:             elbv2types.LoadBalancerTypeEnumApplication,
				},
			},
		},
		tgs: &elbv2.DescribeTargetGroupsOutput{
			TargetGroups: []elbv2types.TargetGroup{
				{TargetGroupArn: aws.String("arn:tg")},
			},
		},
		hErr: errors.New("target health failed"),
	}

	result, err := ScanIdleELBv2(context.Background(), client, "us-east-1", warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Error("skipped LB should not appear in results")
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "ELBv2") {
		t.Errorf("expected ELBv2 health check warning, got %v", warnings)
	}
}
