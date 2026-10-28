package scanner

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

var regionNamePattern = regexp.MustCompile(`^[a-z]{2,4}(-[a-z0-9]+)+-[0-9]+$`)

// IsValidRegionName checks region names before they are used as SDK endpoints.
func IsValidRegionName(name string) bool { return regionNamePattern.MatchString(name) }

// RegionDiscoveryClient is separate from EC2Client so existing scan clients do
// not need the account-level discovery operation.
type RegionDiscoveryClient interface {
	DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
}

// DiscoverEnabledRegions returns sorted, unique regions enabled for the account
// in the client's partition. It never enables regions or treats an empty result
// as complete coverage.
func DiscoverEnabledRegions(ctx context.Context, client RegionDiscoveryClient) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{AllRegions: aws.Bool(false)})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("region discovery returned no enabled regions")
	}
	seen := make(map[string]bool, len(result.Regions))
	var regions []string
	for _, region := range result.Regions {
		if aws.ToString(region.OptInStatus) == "not-opted-in" {
			continue
		}
		name := aws.ToString(region.RegionName)
		if !IsValidRegionName(name) {
			return nil, errors.New("region discovery returned invalid region metadata")
		}
		if !seen[name] {
			seen[name] = true
			regions = append(regions, name)
		}
	}
	if len(regions) == 0 {
		return nil, errors.New("region discovery returned no enabled regions")
	}
	slices.Sort(regions)
	return regions, nil
}
