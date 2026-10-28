package scanner

import (
	"context"
	"errors"

	"github.com/ClusterLedger/OrphanScan/pkg/diagnostic"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type ELBV2Client interface {
	DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DescribeTargetGroups(ctx context.Context, params *elbv2.DescribeTargetGroupsInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error)
	DescribeTargetHealth(ctx context.Context, params *elbv2.DescribeTargetHealthInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error)
}

// ScanIdleELBv2 returns load balancers with no registered targets.
// The returned error covers load balancer listing failures only. Individual
// target group or target health inspection failures are skipped and reported
// through warn when non-nil; incomplete metadata is also skipped. A nil error,
// including with empty findings, does not establish complete coverage. Use
// ScanAWS and inspect its Checks when completion status or unpriced review
// findings are needed.
func ScanIdleELBv2(ctx context.Context, client ELBV2Client, region string, warn WarnFunc) ([]IdleLoadBalancer, error) {
	result, err := scanELBv2(ctx, client, region, warn)
	return result.Idle, err
}

func scanELBv2(ctx context.Context, client ELBV2Client, region string, warn WarnFunc) (loadBalancerResult, error) {
	var result loadBalancerResult
	lbs, err := listELBv2(ctx, client)
	if err != nil {
		return result, err
	}

	for _, lb := range lbs {
		arn := safeString(lb.LoadBalancerArn)
		name := safeString(lb.LoadBalancerName)
		if arn == "" || name == "" {
			result.Errors = append(result.Errors, "load balancer metadata was incomplete")
			continue
		}

		targets, err := inspectTargets(ctx, client, arn)
		if err != nil {
			message := diagnostic.Message(err)
			result.Errors = append(result.Errors, message)
			if warn != nil {
				warn("ELBv2 health check failed: %s", message)
			}
			continue
		}
		result.Inspected++
		if targets.healthy {
			continue
		}

		finding := IdleLoadBalancer{
			Name:   name,
			Type:   lbType(lb.Type),
			Region: region,
			Zones:  len(lb.AvailabilityZones),
		}
		if targets.registered {
			reason := "registered_backends_unhealthy_or_unknown"
			if targets.healthDisabled {
				reason = "health_checks_disabled"
			}
			result.Review = append(result.Review, LoadBalancerReview{IdleLoadBalancer: finding, Reason: reason})
			continue
		}
		if finding.Type == "UNKNOWN" {
			result.Review = append(result.Review, LoadBalancerReview{IdleLoadBalancer: finding, Reason: "unsupported_load_balancer_type"})
			continue
		}
		result.Idle = append(result.Idle, finding)
	}
	return result, nil
}

func listELBv2(ctx context.Context, client ELBV2Client) ([]elbv2types.LoadBalancer, error) {
	var all []elbv2types.LoadBalancer
	input := &elbv2.DescribeLoadBalancersInput{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := client.DescribeLoadBalancers(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, out.LoadBalancers...)
		if out.NextMarker == nil || *out.NextMarker == "" {
			break
		}
		if safeString(input.Marker) == *out.NextMarker {
			return nil, errors.New("load balancer pagination did not advance")
		}
		input.Marker = out.NextMarker
	}
	return all, nil
}

type targetState struct {
	registered     bool
	healthy        bool
	healthDisabled bool
}

func inspectTargets(ctx context.Context, client ELBV2Client, lbArn string) (targetState, error) {
	var state targetState
	groups, err := listTargetGroups(ctx, client, lbArn)
	if err != nil {
		return state, err
	}

	for _, tg := range groups {
		tgArn := safeString(tg.TargetGroupArn)
		if tgArn == "" {
			return state, errors.New("target group metadata was incomplete")
		}

		healthOut, err := client.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: &tgArn,
		})
		if err != nil {
			return state, err
		}
		state.registered = state.registered || len(healthOut.TargetHealthDescriptions) > 0
		if len(healthOut.TargetHealthDescriptions) > 0 && tg.HealthCheckEnabled != nil && !*tg.HealthCheckEnabled {
			state.healthDisabled = true
		}

		for _, desc := range healthOut.TargetHealthDescriptions {
			if desc.TargetHealth != nil && desc.TargetHealth.State == elbv2types.TargetHealthStateEnumHealthy {
				state.healthy = true
			}
		}
	}

	return state, nil
}

func listTargetGroups(ctx context.Context, client ELBV2Client, arn string) ([]elbv2types.TargetGroup, error) {
	var groups []elbv2types.TargetGroup
	input := &elbv2.DescribeTargetGroupsInput{LoadBalancerArn: aws.String(arn)}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := client.DescribeTargetGroups(ctx, input)
		if err != nil {
			return nil, err
		}
		groups = append(groups, out.TargetGroups...)
		if out.NextMarker == nil || *out.NextMarker == "" {
			return groups, nil
		}
		if safeString(input.Marker) == *out.NextMarker {
			return nil, errors.New("target group pagination did not advance")
		}
		input.Marker = out.NextMarker
	}
}

func lbType(t elbv2types.LoadBalancerTypeEnum) string {
	switch t {
	case elbv2types.LoadBalancerTypeEnumApplication:
		return LBTypeALB
	case elbv2types.LoadBalancerTypeEnumNetwork:
		return LBTypeNLB
	case elbv2types.LoadBalancerTypeEnumGateway:
		return LBTypeGWLB
	default:
		return "UNKNOWN"
	}
}
