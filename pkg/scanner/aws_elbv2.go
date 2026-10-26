package scanner

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type ELBV2Client interface {
	DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DescribeTargetGroups(ctx context.Context, params *elbv2.DescribeTargetGroupsInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error)
	DescribeTargetHealth(ctx context.Context, params *elbv2.DescribeTargetHealthInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error)
}

func ScanIdleELBv2(ctx context.Context, client ELBV2Client, region string, warn WarnFunc) ([]IdleLoadBalancer, error) {
	lbs, err := listELBv2(ctx, client)
	if err != nil {
		return nil, err
	}

	var idle []IdleLoadBalancer
	for _, lb := range lbs {
		arn := safeString(lb.LoadBalancerArn)
		name := safeString(lb.LoadBalancerName)
		if arn == "" || name == "" {
			continue
		}

		hasHealthy, err := hasHealthyTarget(ctx, client, arn)
		if err != nil {
			if warn != nil {
				warn("ELBv2 %s health check failed: %v", name, err)
			}
			continue
		}
		if hasHealthy {
			continue
		}

		idle = append(idle, IdleLoadBalancer{
			Name:   name,
			Type:   lbType(lb.Type),
			Region: region,
		})
	}
	return idle, nil
}

func listELBv2(ctx context.Context, client ELBV2Client) ([]elbv2types.LoadBalancer, error) {
	var all []elbv2types.LoadBalancer
	input := &elbv2.DescribeLoadBalancersInput{}
	for {
		out, err := client.DescribeLoadBalancers(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, out.LoadBalancers...)
		if out.NextMarker == nil {
			break
		}
		input.Marker = out.NextMarker
	}
	return all, nil
}

func hasHealthyTarget(ctx context.Context, client ELBV2Client, lbArn string) (bool, error) {
	tgOut, err := client.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	if err != nil {
		return false, err
	}

	if len(tgOut.TargetGroups) == 0 {
		return false, nil
	}

	for _, tg := range tgOut.TargetGroups {
		tgArn := safeString(tg.TargetGroupArn)
		if tgArn == "" {
			continue
		}

		healthOut, err := client.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: &tgArn,
		})
		if err != nil {
			return false, err
		}

		for _, desc := range healthOut.TargetHealthDescriptions {
			if desc.TargetHealth != nil && desc.TargetHealth.State == elbv2types.TargetHealthStateEnumHealthy {
				return true, nil
			}
		}
	}

	return false, nil
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
		return string(t)
	}
}
