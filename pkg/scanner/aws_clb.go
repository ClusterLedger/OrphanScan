package scanner

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
)

type ELBClient interface {
	DescribeLoadBalancers(ctx context.Context, params *elb.DescribeLoadBalancersInput, optFns ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error)
	DescribeInstanceHealth(ctx context.Context, params *elb.DescribeInstanceHealthInput, optFns ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error)
}

type IdleLoadBalancer struct {
	Name   string
	Type   string
	Region string
}

func ScanIdleClassicLBs(ctx context.Context, client ELBClient, region string) ([]IdleLoadBalancer, error) {
	lbs, err := listClassicLBs(ctx, client)
	if err != nil {
		return nil, err
	}

	var idle []IdleLoadBalancer
	for _, lb := range lbs {
		name := safeString(lb.LoadBalancerName)
		if name == "" {
			continue
		}

		healthy, err := hasHealthyClassicInstance(ctx, client, name)
		if err != nil {
			continue
		}

		if healthy {
			continue
		}

		idle = append(idle, IdleLoadBalancer{
			Name:   name,
			Type:   "CLB",
			Region: region,
		})
	}

	return idle, nil
}

func listClassicLBs(ctx context.Context, client ELBClient) ([]elbtypes.LoadBalancerDescription, error) {
	out, err := client.DescribeLoadBalancers(ctx, &elb.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, err
	}

	return out.LoadBalancerDescriptions, nil
}

func hasHealthyClassicInstance(ctx context.Context, client ELBClient, lbName string) (bool, error) {
	out, err := client.DescribeInstanceHealth(ctx, &elb.DescribeInstanceHealthInput{
		LoadBalancerName: aws.String(lbName),
	})
	if err != nil {
		return false, err
	}

	for _, instance := range out.InstanceStates {
		if instance.State != nil && *instance.State == "InService" {
			return true, nil
		}
	}

	return false, nil
}
