package scanner

import (
	"context"
	"errors"

	"github.com/ClusterLedger/OrphanScan/pkg/diagnostic"
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
	Zones  int
}

// LoadBalancerReview describes registered backends whose health merits review.
// These findings are deliberately excluded from estimated waste.
type LoadBalancerReview struct {
	IdleLoadBalancer
	Reason string
}

type loadBalancerResult struct {
	Idle      []IdleLoadBalancer
	Review    []LoadBalancerReview
	Inspected int
	Errors    []string
}

// ScanIdleClassicLBs returns Classic load balancers with no registered instances.
// The returned error covers load balancer listing failures only. Individual
// health inspection failures are skipped and reported through warn when non-nil;
// incomplete metadata is also skipped. A nil error, including with empty
// findings, does not establish complete coverage. Use ScanAWS and inspect its
// Checks when completion status or unpriced review findings are needed.
func ScanIdleClassicLBs(ctx context.Context, client ELBClient, region string, warn WarnFunc) ([]IdleLoadBalancer, error) {
	result, err := scanClassicLBs(ctx, client, region, warn)
	return result.Idle, err
}

func scanClassicLBs(ctx context.Context, client ELBClient, region string, warn WarnFunc) (loadBalancerResult, error) {
	var result loadBalancerResult
	lbs, err := listClassicLBs(ctx, client)
	if err != nil {
		return result, err
	}

	for _, lb := range lbs {
		name := safeString(lb.LoadBalancerName)
		if name == "" {
			result.Errors = append(result.Errors, "load balancer metadata was incomplete")
			continue
		}

		finding := IdleLoadBalancer{Name: name, Type: LBTypeCLB, Region: region}
		if len(lb.Instances) == 0 {
			result.Idle = append(result.Idle, finding)
			result.Inspected++
			continue
		}
		healthy, err := hasHealthyClassicInstance(ctx, client, name)
		if err != nil {
			message := diagnostic.Message(err)
			result.Errors = append(result.Errors, message)
			if warn != nil {
				warn("CLB health check failed: %s", message)
			}
			continue
		}
		result.Inspected++
		if healthy {
			continue
		}
		result.Review = append(result.Review, LoadBalancerReview{
			IdleLoadBalancer: finding,
			Reason:           "registered_backends_unhealthy_or_unknown",
		})
	}

	return result, nil
}

func listClassicLBs(ctx context.Context, client ELBClient) ([]elbtypes.LoadBalancerDescription, error) {
	var all []elbtypes.LoadBalancerDescription
	input := &elb.DescribeLoadBalancersInput{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := client.DescribeLoadBalancers(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, out.LoadBalancerDescriptions...)
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
