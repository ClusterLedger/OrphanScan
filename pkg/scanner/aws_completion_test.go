package scanner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	v2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
)

type completionELBClient struct{ stubELBClient }

func (c *completionELBClient) DescribeInstanceHealth(_ context.Context, in *elb.DescribeInstanceHealthInput, _ ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error) {
	if aws.ToString(in.LoadBalancerName) == "unhealthy" {
		return &elb.DescribeInstanceHealthOutput{InstanceStates: []elbtypes.InstanceState{{State: aws.String("OutOfService")}}}, nil
	}
	return nil, c.hErr
}

func completionClients(resource string, findings bool, failure string) RegionClients {
	clients := completeClients()
	denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "private-account at https://private-cluster.invalid"}
	if resource == "classic_load_balancers" {
		client := &completionELBClient{stubELBClient{lbs: &elb.DescribeLoadBalancersOutput{}, hErr: denied}}
		if findings {
			client.lbs.LoadBalancerDescriptions = []elbtypes.LoadBalancerDescription{
				{LoadBalancerName: aws.String("empty")},
				{LoadBalancerName: aws.String("unhealthy"), Instances: []elbtypes.Instance{{InstanceId: aws.String("i-fixture")}}},
			}
		}
		switch failure {
		case "health":
			client.lbs.LoadBalancerDescriptions = append(client.lbs.LoadBalancerDescriptions, elbtypes.LoadBalancerDescription{LoadBalancerName: aws.String("uninspected"), Instances: []elbtypes.Instance{{InstanceId: aws.String("i-fixture")}}})
		case "metadata":
			client.lbs.LoadBalancerDescriptions = append(client.lbs.LoadBalancerDescriptions, elbtypes.LoadBalancerDescription{})
		}
		clients.ELB = client
		return clients
	}
	client := &pagedV2{
		stubELBV2Client: stubELBV2Client{lbs: &elbv2.DescribeLoadBalancersOutput{}},
		groups: func(in *elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error) {
			return &elbv2.DescribeTargetGroupsOutput{TargetGroups: []v2types.TargetGroup{{TargetGroupArn: in.LoadBalancerArn}}}, nil
		},
		targets: func(in *elbv2.DescribeTargetHealthInput) (*elbv2.DescribeTargetHealthOutput, error) {
			switch aws.ToString(in.TargetGroupArn) {
			case "arn:empty":
				return &elbv2.DescribeTargetHealthOutput{}, nil
			case "arn:unhealthy":
				return &elbv2.DescribeTargetHealthOutput{TargetHealthDescriptions: []v2types.TargetHealthDescription{{TargetHealth: &v2types.TargetHealth{State: v2types.TargetHealthStateEnumUnhealthy}}}}, nil
			default:
				return nil, denied
			}
		},
	}
	if findings {
		client.lbs.LoadBalancers = []v2types.LoadBalancer{
			{LoadBalancerArn: aws.String("arn:empty"), LoadBalancerName: aws.String("empty"), Type: v2types.LoadBalancerTypeEnumApplication},
			{LoadBalancerArn: aws.String("arn:unhealthy"), LoadBalancerName: aws.String("unhealthy"), Type: v2types.LoadBalancerTypeEnumApplication},
		}
	}
	switch failure {
	case "health":
		client.lbs.LoadBalancers = append(client.lbs.LoadBalancers, v2types.LoadBalancer{LoadBalancerArn: aws.String("arn:uninspected"), LoadBalancerName: aws.String("uninspected"), Type: v2types.LoadBalancerTypeEnumApplication})
	case "metadata":
		client.lbs.LoadBalancers = append(client.lbs.LoadBalancers, v2types.LoadBalancer{})
	}
	clients.ELBV2 = client
	return clients
}

func TestScanAWSLoadBalancerCompletion(t *testing.T) {
	for _, resource := range []string{"classic_load_balancers", "elbv2_load_balancers"} {
		for _, tt := range []struct {
			name, failure, status, diagnostic string
			findings                          bool
		}{
			{name: "empty scan is complete", status: StatusComplete},
			{name: "findings and reviews are complete", findings: true, status: StatusComplete},
			{name: "health failure retains findings and reviews", findings: true, failure: "health", status: StatusPartial, diagnostic: "Permission denied."},
			{name: "all health inspections fail", failure: "health", status: StatusFailed, diagnostic: "Permission denied."},
			{name: "metadata failure retains findings and reviews", findings: true, failure: "metadata", status: StatusPartial, diagnostic: "load balancer metadata was incomplete"},
			{name: "all metadata is incomplete", failure: "metadata", status: StatusFailed, diagnostic: "load balancer metadata was incomplete"},
		} {
			t.Run(resource+"/"+tt.name, func(t *testing.T) {
				var warnings []string
				result := ScanAWS(context.Background(), aws.Config{}, []string{"us-east-1"}, func(aws.Config, string) RegionClients {
					return completionClients(resource, tt.findings, tt.failure)
				}, func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) })
				if len(result.Checks) != 4 {
					t.Fatalf("checks=%+v", result.Checks)
				}
				checks := make(map[string]Check)
				for _, check := range result.Checks {
					checks[check.Resource] = check
					if check.Source != SourceAWS || check.Region != "us-east-1" {
						t.Fatalf("incorrect check scope: %+v", check)
					}
				}
				for _, name := range []string{"ebs_volumes", "elastic_ips", "classic_load_balancers", "elbv2_load_balancers"} {
					want := StatusComplete
					if name == resource {
						want = tt.status
					}
					if checks[name].Status != want {
						t.Fatalf("%s status=%q, want %q; checks=%+v", name, checks[name].Status, want, result.Checks)
					}
				}
				check := checks[resource]
				if !strings.Contains(check.Error, tt.diagnostic) || (tt.diagnostic == "" && check.Error != "") {
					t.Fatalf("diagnostic=%q, want %q", check.Error, tt.diagnostic)
				}
				wantCount, wantWarnings, overall := 0, 0, StatusComplete
				if tt.findings {
					wantCount = 1
				}
				if tt.failure != "" {
					wantWarnings, overall = 1, StatusPartial
				}
				if len(result.LBs) != wantCount || len(result.LBReviews) != wantCount || len(warnings) != wantWarnings || OverallStatus(result.Checks) != overall {
					t.Fatalf("unexpected aggregate result: %+v warnings=%v", result, warnings)
				}
				if tt.findings && (result.LBs[0].Name != "empty" || result.LBReviews[0].Name != "unhealthy" || result.LBReviews[0].Reason != "registered_backends_unhealthy_or_unknown") {
					t.Fatalf("retained the wrong findings: %+v", result)
				}
				for _, private := range []string{"private-account", "private-cluster.invalid"} {
					if strings.Contains(fmt.Sprint(result.Checks)+strings.Join(warnings, "\n"), private) {
						t.Fatalf("provider details leaked into checks or warnings: %+v %v", result.Checks, warnings)
					}
				}
			})
		}
	}
}
