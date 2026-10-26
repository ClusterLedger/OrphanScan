package scanner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockEC2Client struct {
	volumes    []ec2.DescribeVolumesOutput
	volumesErr error
	addresses  *ec2.DescribeAddressesOutput
	addrErr    error
	callCount  int
}

func (m *mockEC2Client) DescribeVolumes(_ context.Context, input *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	if m.volumesErr != nil {
		return nil, m.volumesErr
	}
	idx := m.callCount
	if idx >= len(m.volumes) {
		return &ec2.DescribeVolumesOutput{}, nil
	}
	m.callCount++
	return &m.volumes[idx], nil
}

func (m *mockEC2Client) DescribeAddresses(_ context.Context, _ *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	if m.addrErr != nil {
		return nil, m.addrErr
	}
	return m.addresses, nil
}

func TestScanUnattachedVolumes(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	recent := time.Now().Add(-2 * time.Hour)

	client := &mockEC2Client{
		volumes: []ec2.DescribeVolumesOutput{{
			Volumes: []types.Volume{
				{VolumeId: aws.String("vol-old"), Size: aws.Int32(100), CreateTime: &old},
				{VolumeId: aws.String("vol-new"), Size: aws.Int32(50), CreateTime: &recent},
			},
		}},
	}
	result, err := ScanUnattachedVolumes(context.Background(), client, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].VolumeID != "vol-old" {
		t.Fatalf("expected 1 old orphan, got %v", result)
	}
}

func TestScanUnattachedVolumes_Pagination(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	token := "page2"
	client := &mockEC2Client{
		volumes: []ec2.DescribeVolumesOutput{
			{
				Volumes:   []types.Volume{{VolumeId: aws.String("vol-1"), Size: aws.Int32(10), CreateTime: &old}},
				NextToken: &token,
			},
			{
				Volumes: []types.Volume{{VolumeId: aws.String("vol-2"), Size: aws.Int32(20), CreateTime: &old}},
			},
		},
	}
	result, err := ScanUnattachedVolumes(context.Background(), client, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 volumes across pages, got %d", len(result))
	}
}

func TestScanUnattachedVolumes_Error(t *testing.T) {
	client := &mockEC2Client{volumesErr: errors.New("access denied")}
	_, err := ScanUnattachedVolumes(context.Background(), client, "us-east-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestScanUnassociatedEIPs(t *testing.T) {
	client := &mockEC2Client{
		addresses: &ec2.DescribeAddressesOutput{
			Addresses: []types.Address{
				{AllocationId: aws.String("eip-1"), PublicIp: aws.String("1.2.3.4")},
				{AllocationId: aws.String("eip-2"), PublicIp: aws.String("5.6.7.8"), InstanceId: aws.String("i-123")},
			},
		},
	}
	result, err := ScanUnassociatedEIPs(context.Background(), client, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].AllocationID != "eip-1" {
		t.Fatalf("expected 1 unassociated EIP, got %v", result)
	}
}

func TestScanUnassociatedEIPs_Error(t *testing.T) {
	client := &mockEC2Client{addrErr: errors.New("throttled")}
	_, err := ScanUnassociatedEIPs(context.Background(), client, "us-east-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestScanUnassociatedEIPs_Empty(t *testing.T) {
	client := &mockEC2Client{
		addresses: &ec2.DescribeAddressesOutput{},
	}
	result, err := ScanUnassociatedEIPs(context.Background(), client, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 EIPs, got %d", len(result))
	}
}
