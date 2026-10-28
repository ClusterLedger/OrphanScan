package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type discoveryFunc func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error)

func (f discoveryFunc) DescribeRegions(ctx context.Context, input *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return f(ctx, input)
}

func discoveryResponse(names ...string) *ec2.DescribeRegionsOutput {
	result := &ec2.DescribeRegionsOutput{}
	for _, name := range names {
		result.Regions = append(result.Regions, types.Region{RegionName: aws.String(name)})
	}
	return result
}

func TestAWSRegionSelection(t *testing.T) {
	for _, tt := range []struct {
		name, configured, endpoint               string
		args, discovered, wantRegions, wantLoads []string
	}{
		{name: "explicit regions", configured: "eu-west-1", args: []string{"--regions=us-west-2,ap-south-1,us-west-2"}, wantRegions: []string{"ap-south-1", "us-west-2"}, wantLoads: []string{"us-west-2"}},
		{name: "configured SDK region", configured: "eu-west-1", wantRegions: []string{"eu-west-1"}, wantLoads: []string{""}},
		{name: "automatic discovery", endpoint: "us-east-1", discovered: []string{"us-west-2", "ap-south-1", "us-west-2"}, wantRegions: []string{"ap-south-1", "us-west-2"}, wantLoads: []string{"", "us-east-1"}},
		{name: "explicit all without configured region", args: []string{"--all-regions"}, endpoint: "us-east-1", discovered: []string{"us-west-2"}, wantRegions: []string{"us-west-2"}, wantLoads: []string{"", "us-east-1"}},
		{name: "explicit all overrides configured scan region", configured: "ap-south-1", args: []string{"--all-regions"}, endpoint: "ap-south-1", discovered: []string{"us-west-2", "eu-west-1"}, wantRegions: []string{"eu-west-1", "us-west-2"}, wantLoads: []string{""}},
		{name: "disabled all flag", configured: "eu-west-1", args: []string{"--all-regions=false"}, wantRegions: []string{"eu-west-1"}, wantLoads: []string{""}},
		{name: "GovCloud partition", configured: "us-gov-west-1", args: []string{"--all-regions"}, endpoint: "us-gov-west-1", discovered: []string{"us-gov-west-1", "us-gov-east-1"}, wantRegions: []string{"us-gov-east-1", "us-gov-west-1"}, wantLoads: []string{""}},
		{name: "China partition", configured: "cn-north-1", args: []string{"--all-regions"}, endpoint: "cn-north-1", discovered: []string{"cn-northwest-1", "cn-north-1"}, wantRegions: []string{"cn-north-1", "cn-northwest-1"}, wantLoads: []string{""}},
		{name: "sovereign partition", configured: "eusc-de-east-1", args: []string{"--all-regions"}, endpoint: "eusc-de-east-1", discovered: []string{"eusc-de-east-1"}, wantRegions: []string{"eusc-de-east-1"}, wantLoads: []string{""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := successfulDependencies()
			var loads, scanned []string
			var mu sync.Mutex
			var retrievals, discoveries atomic.Int32
			deps.awsConfig = func(_ context.Context, profile, region string) (aws.Config, error) {
				if profile != "audit" {
					t.Errorf("profile=%q", profile)
				}
				mu.Lock()
				loads = append(loads, region)
				mu.Unlock()
				if region == "" {
					region = tt.configured
				}
				return aws.Config{Region: region, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					retrievals.Add(1)
					if region == "" {
						return aws.Credentials{}, errors.New("credential client was initialized without a region")
					}
					return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
				})}, nil
			}
			deps.regionClient = func(cfg aws.Config) scanner.RegionDiscoveryClient {
				if cfg.Region != tt.endpoint {
					t.Errorf("discovery endpoint=%q, want %q", cfg.Region, tt.endpoint)
				}
				return discoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
					discoveries.Add(1)
					return discoveryResponse(tt.discovered...), nil
				})
			}
			factory := deps.awsClients
			deps.awsClients = func(cfg aws.Config, region string) scanner.RegionClients {
				mu.Lock()
				scanned = append(scanned, region)
				mu.Unlock()
				return factory(cfg, region)
			}
			var stdout, stderr bytes.Buffer
			args := append([]string{"--sources=aws", "--output=json", "--aws-profile=audit"}, tt.args...)
			if code := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), deps); code != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			mu.Lock()
			defer mu.Unlock()
			slices.Sort(scanned)
			if !reflect.DeepEqual(loads, tt.wantLoads) || !reflect.DeepEqual(scanned, tt.wantRegions) || retrievals.Load() != 1 {
				t.Fatalf("loads=%v scanned=%v retrievals=%d", loads, scanned, retrievals.Load())
			}
			wantDiscoveries := int32(0)
			if tt.endpoint != "" {
				wantDiscoveries = 1
			}
			if discoveries.Load() != wantDiscoveries {
				t.Fatalf("discovery calls=%d, want %d", discoveries.Load(), wantDiscoveries)
			}
			var result struct {
				Checks []scanner.Check `json:"checks"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Checks) != 4*len(tt.wantRegions) {
				t.Fatalf("resource coverage=%v", result.Checks)
			}
		})
	}
}

func TestKubernetesOnlyDoesNotDiscoverAWSRegions(t *testing.T) {
	deps := successfulDependencies()
	deps.awsConfig, deps.awsClients, deps.regionClient = nil, nil, nil
	var stdout, stderr bytes.Buffer
	if code := runCommand(context.Background(), []string{"--sources=kubernetes", "--all-regions"}, testRuntime(&stdout, &stderr), deps); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestDiscoveryDoesNotCountAsUsableResults(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "private-account-and-resource"}
	for _, tt := range []struct {
		name                      string
		failDiscovery, kubernetes bool
		code, checks              int
	}{
		{name: "discovery denied", failDiscovery: true, code: 1, checks: 1},
		{name: "all resource checks denied", code: 1, checks: 8},
		{name: "discovery denied with Kubernetes results", failDiscovery: true, kubernetes: true, code: 3, checks: 2},
		{name: "all resource checks denied with Kubernetes results", kubernetes: true, code: 3, checks: 9},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := successfulDependencies()
			deps.regionClient = func(aws.Config) scanner.RegionDiscoveryClient {
				return discoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
					if tt.failDiscovery {
						return nil, denied
					}
					return discoveryResponse("us-west-2", "us-east-1"), nil
				})
			}
			var resourceCalls atomic.Int32
			deps.awsClients = func(aws.Config, string) scanner.RegionClients {
				resourceCalls.Add(1)
				return scanner.RegionClients{EC2: &ec2Stub{denied, denied}, ELB: &clbStub{denied}, ELBV2: &v2Stub{denied}}
			}
			deps.kubeClient = func(string, string) (scanner.KubeClient, error) {
				return &kubeStub{pvcs: []corev1.PersistentVolumeClaim{{
					ObjectMeta: metav1.ObjectMeta{Name: "private-pvc", Namespace: "private-namespace", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
					Spec:       corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")}}},
					Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
				}}}, nil
			}
			sender := &testSender{}
			preferencePath := filepath.Join(t.TempDir(), "telemetry.json")
			deps.usage = func(Config, commandRuntime) (*telemetry.Reporter, error) {
				return telemetry.NewReporter(sender, telemetry.Options{Mode: "on", PreferencePath: preferencePath})
			}
			sources := "aws"
			if tt.kubernetes {
				sources += ",kubernetes"
			}
			var stdout, stderr bytes.Buffer
			args := []string{"--sources=" + sources, "--all-regions", "--output=json"}
			if code := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), deps); code != tt.code {
				t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", code, tt.code, stdout.String(), stderr.String())
			}
			var result struct {
				Checks []scanner.Check `json:"checks"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Checks) != tt.checks || strings.Contains(stdout.String()+stderr.String(), "private-") {
				t.Fatalf("incorrect or unsafe report: %s %s", stdout.String(), stderr.String())
			}
			if tt.failDiscovery {
				if resourceCalls.Load() != 0 || !strings.Contains(stderr.String(), "ec2:DescribeRegions") {
					t.Fatalf("discovery failure narrowed coverage: calls=%d stderr=%s", resourceCalls.Load(), stderr.String())
				}
				for _, check := range result.Checks {
					if check.Source == scanner.SourceAWS && (check.Resource != "configuration" || check.Status != scanner.StatusFailed) {
						t.Fatalf("discovery failure check=%+v", check)
					}
				}
			}
			if tt.kubernetes {
				if len(sender.costs) != 1 || sender.costs[0] != 8 {
					t.Fatalf("failed AWS discovery/checks changed telemetry estimate: %v", sender.costs)
				}
			} else if len(sender.costs) != 0 {
				t.Fatalf("failed scan reported telemetry: %v", sender.costs)
			}
		})
	}
}

func TestDiscoveryTimeoutAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		deps := successfulDependencies()
		deps.regionClient = func(aws.Config) scanner.RegionDiscoveryClient {
			return discoveryFunc(func(ctx context.Context, _ *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
				if cancelled {
					cancel()
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
		}
		factory := deps.awsClients
		deps.awsClients = func(cfg aws.Config, region string) scanner.RegionClients {
			t.Error("cancelled discovery started resource scans")
			return factory(cfg, region)
		}
		var stdout, stderr bytes.Buffer
		args := []string{"--sources=aws", "--all-regions", "--output=json", "--timeout=20ms"}
		if cancelled {
			args[len(args)-1] = "--timeout=5s"
		}
		code := runCommand(ctx, args, testRuntime(&stdout, &stderr), deps)
		cancel()
		want, message := 1, "timed out"
		if cancelled {
			want, message = 130, "interrupted"
		}
		if code != want || !strings.Contains(stdout.String(), message) {
			t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	}
}

func TestDiscoverySetupFailuresDoNotContactProviders(t *testing.T) {
	for _, name := range []string{"initial load failure", "fallback load failure", "invalid configured region", "invalid forced discovery endpoint"} {
		t.Run(name, func(t *testing.T) {
			deps := successfulDependencies()
			private := "https://private-account.invalid"
			deps.awsConfig = func(_ context.Context, _, region string) (aws.Config, error) {
				if name == "initial load failure" || (name == "fallback load failure" && region != "") {
					return aws.Config{}, errors.New(private)
				}
				if strings.HasPrefix(name, "invalid") {
					region = private
				}
				return aws.Config{Region: region, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					t.Error("invalid configuration resolved credentials")
					return aws.Credentials{}, errors.New("must not run")
				})}, nil
			}
			deps.regionClient = func(aws.Config) scanner.RegionDiscoveryClient {
				t.Error("invalid configuration created a discovery client")
				return discoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
					return nil, errors.New("must not run")
				})
			}
			var stdout, stderr bytes.Buffer
			args := []string{"--sources=aws", "--output=json"}
			if name == "invalid forced discovery endpoint" {
				args = append(args, "--all-regions")
			}
			if code := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), deps); code != 1 || strings.Contains(stdout.String()+stderr.String(), private) {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
		})
	}
}
