package scanner

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

const DefaultConcurrency = 4

// AWSScanResult retains successful findings and the status of every check.
type AWSScanResult struct {
	Volumes   []OrphanedVolume
	EIPs      []OrphanedEIP
	LBs       []IdleLoadBalancer
	LBReviews []LoadBalancerReview
	Checks    []Check
}

type RegionClients struct {
	EC2   EC2Client
	ELB   ELBClient
	ELBV2 ELBV2Client
}

type ClientFactory func(cfg aws.Config, region string) RegionClients

func DefaultClientFactory(cfg aws.Config, region string) RegionClients {
	regional := cfg.Copy()
	regional.Region = region
	return RegionClients{
		EC2: ec2.NewFromConfig(regional), ELB: elb.NewFromConfig(regional), ELBV2: elbv2.NewFromConfig(regional),
	}
}

type WarnFunc func(format string, args ...any)

func ScanAWS(ctx context.Context, cfg aws.Config, regions []string, factory ClientFactory, warn WarnFunc) AWSScanResult {
	return ScanAWSWithConcurrency(ctx, cfg, regions, factory, warn, DefaultConcurrency)
}

// ScanAWSWithConcurrency bounds concurrent regions. Each region has four
// independent checks. Duplicate regions run once.
func ScanAWSWithConcurrency(ctx context.Context, cfg aws.Config, regions []string, factory ClientFactory, warn WarnFunc, concurrency int) AWSScanResult {
	if factory == nil {
		factory = DefaultClientFactory
	}
	if concurrency < 1 {
		concurrency = DefaultConcurrency
	}
	jobs := make(chan string, len(regions))
	seen := make(map[string]bool)
	for _, region := range regions {
		if !seen[region] {
			jobs <- region
			seen[region] = true
		}
	}
	close(jobs)
	var warnMu sync.Mutex
	if warn != nil {
		callback := warn
		warn = func(format string, args ...any) {
			warnMu.Lock()
			defer warnMu.Unlock()
			callback(format, args...)
		}
	}
	var result AWSScanResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range min(concurrency, len(seen)) {
		wg.Go(func() {
			for region := range jobs {
				var partial AWSScanResult
				if err := ctx.Err(); err != nil {
					for _, resource := range []string{"ebs_volumes", "elastic_ips", "classic_load_balancers", "elbv2_load_balancers"} {
						partial.Checks = append(partial.Checks, newCheck(resource, region, err))
					}
				} else {
					partial = scanRegion(ctx, factory(cfg, region), region, warn)
				}
				mu.Lock()
				result.Volumes = append(result.Volumes, partial.Volumes...)
				result.EIPs = append(result.EIPs, partial.EIPs...)
				result.LBs = append(result.LBs, partial.LBs...)
				result.LBReviews = append(result.LBReviews, partial.LBReviews...)
				result.Checks = append(result.Checks, partial.Checks...)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return result
}

func scanRegion(ctx context.Context, clients RegionClients, region string, warn WarnFunc) AWSScanResult {
	var result AWSScanResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	addCheck := func(check Check) {
		result.Checks = append(result.Checks, check)
		if check.Error != "" && warn != nil {
			warn("region %s %s: %s", region, check.Resource, check.Error)
		}
	}
	wg.Go(func() {
		volumes, err := ScanUnattachedVolumes(ctx, clients.EC2, region)
		mu.Lock()
		defer mu.Unlock()
		result.Volumes = volumes
		addCheck(newCheck("ebs_volumes", region, err))
	})
	wg.Go(func() {
		eips, err := ScanUnassociatedEIPs(ctx, clients.EC2, region)
		mu.Lock()
		defer mu.Unlock()
		result.EIPs = eips
		addCheck(newCheck("elastic_ips", region, err))
	})
	for _, task := range []struct {
		resource string
		scan     func() (loadBalancerResult, error)
	}{
		{"classic_load_balancers", func() (loadBalancerResult, error) { return scanClassicLBs(ctx, clients.ELB, region, nil) }},
		{"elbv2_load_balancers", func() (loadBalancerResult, error) { return scanELBv2(ctx, clients.ELBV2, region, nil) }},
	} {
		wg.Go(func() {
			lbs, err := task.scan()
			check := newCheck(task.resource, region, err)
			if err == nil && len(lbs.Errors) > 0 {
				check.Status = StatusPartial
				if lbs.Inspected == 0 {
					check.Status = StatusFailed
				}
				check.Error = lbs.Errors[0]
			}
			mu.Lock()
			defer mu.Unlock()
			result.LBs = append(result.LBs, lbs.Idle...)
			result.LBReviews = append(result.LBReviews, lbs.Review...)
			addCheck(check)
		})
	}
	wg.Wait()
	return result
}
