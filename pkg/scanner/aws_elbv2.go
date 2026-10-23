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

func ScanIdleELBv2(ctx context.Context, client ELBV2Client, region string) ([]IdleLoadBalancer, error) {
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
			continue // Step 22: log [WARN]
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
	out, err := client.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, err
	}
	return out.LoadBalancers, nil
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
		return "ALB"
	case elbv2types.LoadBalancerTypeEnumNetwork:
		return "NLB"
	case elbv2types.LoadBalancerTypeEnumGateway:
		return "GWLB"
	default:
		return string(t)
	}
}
