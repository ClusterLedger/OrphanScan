package scanner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type regionDiscoveryFunc func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error)

func (f regionDiscoveryFunc) DescribeRegions(ctx context.Context, input *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return f(ctx, input)
}

func TestDiscoverEnabledRegions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		response  *ec2.DescribeRegionsOutput
		want      []string
		wantError bool
	}{
		{
			name: "enabled regions sorted and deduplicated",
			response: &ec2.DescribeRegionsOutput{Regions: []types.Region{
				{RegionName: aws.String("us-west-2"), OptInStatus: aws.String("opt-in-not-required")},
				{RegionName: aws.String("ap-south-2"), OptInStatus: aws.String("opted-in")},
				{RegionName: aws.String("us-west-2")},
				{RegionName: aws.String("ap-east-1"), OptInStatus: aws.String("not-opted-in")},
			}},
			want: []string{"ap-south-2", "us-west-2"},
		},
		{name: "nil response", wantError: true},
		{name: "empty response", response: &ec2.DescribeRegionsOutput{}, wantError: true},
		{name: "disabled only", response: &ec2.DescribeRegionsOutput{Regions: []types.Region{{RegionName: aws.String("ap-east-1"), OptInStatus: aws.String("not-opted-in")}}}, wantError: true},
		{name: "missing name", response: &ec2.DescribeRegionsOutput{Regions: []types.Region{{}}}, wantError: true},
		{name: "invalid name cannot narrow coverage", response: &ec2.DescribeRegionsOutput{Regions: []types.Region{{RegionName: aws.String("us-east-1")}, {RegionName: aws.String("https://private.invalid")}}}, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := regionDiscoveryFunc(func(_ context.Context, input *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
				if input.AllRegions == nil || *input.AllRegions {
					t.Fatal("discovery must explicitly exclude disabled regions")
				}
				return tt.response, nil
			})
			got, err := DiscoverEnabledRegions(context.Background(), client)
			if (err != nil) != tt.wantError || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("regions=%v err=%v, want regions=%v error=%v", got, err, tt.want, tt.wantError)
			}
			if err != nil && strings.Contains(err.Error(), "private.invalid") {
				t.Fatal("invalid metadata leaked into the diagnostic")
			}
		})
	}
}

func TestRegionDiscoveryProviderErrors(t *testing.T) {
	want := errors.New("provider request failed")
	client := regionDiscoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) { return nil, want })
	if regions, err := DiscoverEnabledRegions(context.Background(), client); !errors.Is(err, want) || regions != nil {
		t.Fatalf("regions=%v err=%v", regions, err)
	}
}

func TestRegionDiscoveryCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}
		cancel()
		client := regionDiscoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
			t.Fatal("cancelled discovery contacted the API")
			return nil, nil
		})
		if regions, err := DiscoverEnabledRegions(ctx, client); !errors.Is(err, ctx.Err()) || regions != nil {
			t.Fatalf("regions=%v err=%v", regions, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := regionDiscoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
		cancel()
		return &ec2.DescribeRegionsOutput{Regions: []types.Region{{RegionName: aws.String("us-east-1")}}}, nil
	})
	if regions, err := DiscoverEnabledRegions(ctx, client); !errors.Is(err, context.Canceled) || regions != nil {
		t.Fatalf("late discovery returned usable regions: %v %v", regions, err)
	}
}
