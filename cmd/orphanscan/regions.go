package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
)

const defaultDiscoveryRegion = "us-east-1"

type awsScanConfig struct {
	config   aws.Config
	regions  []string
	discover bool
}

func loadAWSScanConfig(ctx context.Context, cfg Config, deps dependencies) (awsScanConfig, error) {
	awsCfg, err := deps.awsConfig(ctx, cfg.AWSProfile, firstOrEmpty(cfg.Regions))
	setup := awsScanConfig{config: awsCfg, regions: cfg.Regions}

	if err != nil {
		return setup, err
	}
	setup.discover = cfg.AllRegions || (len(cfg.Regions) == 0 && awsCfg.Region == "")
	if setup.discover && awsCfg.Region == "" {
		// Credential providers can capture regional clients during loading.
		// Reload instead of mutating Region after those clients were created.
		setup.config, err = deps.awsConfig(ctx, cfg.AWSProfile, defaultDiscoveryRegion)
	} else if len(setup.regions) == 0 && !setup.discover {
		setup.regions = []string{awsCfg.Region}
	}

	return setup, err
}
