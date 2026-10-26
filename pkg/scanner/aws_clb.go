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

const classicInstanceInService = "InService"

// Load balancer type constants used across scanner and pricing.
const (
	LBTypeCLB  = "CLB"
	LBTypeALB  = "ALB"
	LBTypeNLB  = "NLB"
	LBTypeGWLB = "GWLB"
)

type IdleLoadBalancer struct {
	Name   string
	Type   string
	Region string
}

func ScanIdleClassicLBs(ctx context.Context, client ELBClient, region string, warn WarnFunc) ([]IdleLoadBalancer, error) {
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
			if warn != nil {
				warn("CLB %s health check failed: %v", name, err)
			}
			continue
		}

		if healthy {
			continue
		}

		idle = append(idle, IdleLoadBalancer{
			Name:   name,
			Type:   LBTypeCLB,
			Region: region,
		})
	}

	return idle, nil
}

func listClassicLBs(ctx context.Context, client ELBClient) ([]elbtypes.LoadBalancerDescription, error) {
	var all []elbtypes.LoadBalancerDescription
	input := &elb.DescribeLoadBalancersInput{}
	for {
		out, err := client.DescribeLoadBalancers(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, out.LoadBalancerDescriptions...)
		if out.NextMarker == nil {
			break
		}
		input.Marker = out.NextMarker
	}
	return all, nil
}

func hasHealthyClassicInstance(ctx context.Context, client ELBClient, lbName string) (bool, error) {
	out, err := client.DescribeInstanceHealth(ctx, &elb.DescribeInstanceHealthInput{
		LoadBalancerName: aws.String(lbName),
	})
	if err != nil {
		return false, err
	}

	for _, instance := range out.InstanceStates {
		if instance.State != nil && *instance.State == classicInstanceInService {
			return true, nil
		}
	}

	return false, nil
}
