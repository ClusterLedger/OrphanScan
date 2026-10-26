package scanner

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// AWSScanResult holds all orphaned AWS resources across regions.
type AWSScanResult struct {
	Volumes []OrphanedVolume
	EIPs    []OrphanedEIP
	LBs     []IdleLoadBalancer
}

// RegionClients holds the three clients for a single region.
type RegionClients struct {
	EC2   EC2Client
	ELB   ELBClient
	ELBV2 ELBV2Client
}

// ClientFactory builds region-scoped clients from a base AWS config.
type ClientFactory func(cfg aws.Config, region string) RegionClients

// DefaultClientFactory creates real AWS SDK clients for a region.
func DefaultClientFactory(cfg aws.Config, region string) RegionClients {
	regional := cfg.Copy()
	regional.Region = region

	return RegionClients{
		EC2:   ec2.NewFromConfig(regional),
		ELB:   elb.NewFromConfig(regional),
		ELBV2: elbv2.NewFromConfig(regional),
	}
}

// WarnFunc is a callback for non-fatal warnings during scanning.
type WarnFunc func(format string, args ...any)

func ScanAWS(ctx context.Context, cfg aws.Config, regions []string, factory ClientFactory, warn WarnFunc) AWSScanResult {
	if factory == nil {
		factory = DefaultClientFactory
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result AWSScanResult
	)

	for _, region := range regions {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()

			clients := factory(cfg, region)
			partial := scanRegion(ctx, clients, region, warn)

			mu.Lock()
			result.Volumes = append(result.Volumes, partial.Volumes...)
			result.EIPs = append(result.EIPs, partial.EIPs...)
			result.LBs = append(result.LBs, partial.LBs...)
			mu.Unlock()
		}(region)
	}

	wg.Wait()
	return result
}

func scanRegion(ctx context.Context, clients RegionClients, region string, warn WarnFunc) AWSScanResult {
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		result AWSScanResult
	)

	wg.Add(4)

	go func() {
		defer wg.Done()
		volumes, err := ScanUnattachedVolumes(ctx, clients.EC2, region)
		if err != nil {
			if warn != nil {
				warn("region %s volumes: %v", region, err)
			}
			return
		}
		mu.Lock()
		result.Volumes = volumes
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		eips, err := ScanUnassociatedEIPs(ctx, clients.EC2, region)
		if err != nil {
			if warn != nil {
				warn("region %s eips: %v", region, err)
			}
			return
		}
		mu.Lock()
		result.EIPs = eips
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		clbs, err := ScanIdleClassicLBs(ctx, clients.ELB, region, warn)
		if err != nil {
			if warn != nil {
				warn("region %s classic LBs: %v", region, err)
			}
			return
		}
		mu.Lock()
		result.LBs = append(result.LBs, clbs...)
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		elbv2LBs, err := ScanIdleELBv2(ctx, clients.ELBV2, region, warn)
		if err != nil {
			if warn != nil {
				warn("region %s ELBv2: %v", region, err)
			}
			return
		}
		mu.Lock()
		result.LBs = append(result.LBs, elbv2LBs...)
		mu.Unlock()
	}()

	wg.Wait()
	return result
}
