package scanner

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type EC2Client interface {
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DescribeAddresses(ctx context.Context, params *ec2.DescribeAddressesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error)
}

type OrphanedVolume struct {
	VolumeID string
	Region   string
	SizeGB   int32
	Age      time.Duration
}

type OrphanedEIP struct {
	AllocationID string
	PublicIP     string
	Region       string
}

func ScanUnattachedVolumes(ctx context.Context, client EC2Client, region string) ([]OrphanedVolume, error) {
	volumes, err := listAvailableVolumes(ctx, client)
	if err != nil {
		return nil, err
	}
	return filterOrphanedVolumes(volumes, region, time.Now()), nil
}

func listAvailableVolumes(ctx context.Context, client EC2Client) ([]types.Volume, error) {
	var all []types.Volume
	input := &ec2.DescribeVolumesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("status"),
				Values: []string{"available"},
			},
		},
	}
	for {
		out, err := client.DescribeVolumes(ctx, input)
		if err != nil {
			return nil, err
		}
		all = append(all, out.Volumes...)
		if out.NextToken == nil {
			break
		}
		input.NextToken = out.NextToken
	}
	return all, nil
}

func filterOrphanedVolumes(volumes []types.Volume, region string, now time.Time) []OrphanedVolume {
	var orphaned []OrphanedVolume

	for _, vol := range volumes {
		age, ok := volumeAge(vol, now)
		if !ok || age < orphanAgeThreshold {
			continue
		}
		orphaned = append(orphaned, toOrphanedVolume(vol, region, age))
	}
	return orphaned
}

func volumeAge(vol types.Volume, now time.Time) (time.Duration, bool) {
	if vol.CreateTime == nil {
		return 0, false
	}
	return now.Sub(*vol.CreateTime), true
}

func toOrphanedVolume(vol types.Volume, region string, age time.Duration) OrphanedVolume {
	return OrphanedVolume{
		VolumeID: safeString(vol.VolumeId),
		Region:   region,
		SizeGB:   safeInt32(vol.Size),
		Age:      age,
	}
}

func safeString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func safeInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func ScanUnassociatedEIPs(ctx context.Context, client EC2Client, region string) ([]OrphanedEIP, error) {
	addrs, err := listAddresses(ctx, client)
	if err != nil {
		return nil, err
	}
	return filterUnassociatedEIPs(addrs, region), nil
}

func listAddresses(ctx context.Context, client EC2Client) ([]types.Address, error) {
	out, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}
	return out.Addresses, nil
}

func filterUnassociatedEIPs(addrs []types.Address, region string) []OrphanedEIP {
	var orphaned []OrphanedEIP
	for _, addr := range addrs {
		if isAssociated(addr) {
			continue
		}
		orphaned = append(orphaned, toOrphanedEIP(addr, region))
	}
	return orphaned
}

func isAssociated(addr types.Address) bool {
	return addr.InstanceId != nil || addr.NetworkInterfaceId != nil
}

func toOrphanedEIP(addr types.Address, region string) OrphanedEIP {
	return OrphanedEIP{
		AllocationID: safeString(addr.AllocationId),
		PublicIP:     safeString(addr.PublicIp),
		Region:       region,
	}
}
