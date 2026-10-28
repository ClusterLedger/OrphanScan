package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/smithy-go"
	corev1 "k8s.io/api/core/v1"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func testRuntime(stdout, stderr io.Writer) commandRuntime {
	return commandRuntime{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr}
}

type kubeStub struct {
	pvcs    []corev1.PersistentVolumeClaim
	err     error
	wait    bool
	started chan struct{}
	release <-chan struct{}
}

func (k *kubeStub) ListPVCs(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
	if k.started != nil {
		close(k.started)
	}
	if k.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if k.release != nil {
		select {
		case <-k.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return k.pvcs, k.err
}
func (k *kubeStub) ListPods(ctx context.Context) ([]corev1.Pod, error) { return nil, k.err }

type ec2Stub struct{ volumeErr, errorIP error }

func (c *ec2Stub) DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{}, c.volumeErr
}
func (c *ec2Stub) DescribeAddresses(context.Context, *ec2.DescribeAddressesInput, ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	return &ec2.DescribeAddressesOutput{}, c.errorIP
}

type clbStub struct{ err error }

func (c *clbStub) DescribeLoadBalancers(context.Context, *elb.DescribeLoadBalancersInput, ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error) {
	return &elb.DescribeLoadBalancersOutput{}, c.err
}
func (c *clbStub) DescribeInstanceHealth(context.Context, *elb.DescribeInstanceHealthInput, ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error) {
	return &elb.DescribeInstanceHealthOutput{}, c.err
}

type v2Stub struct{ err error }

func (c *v2Stub) DescribeLoadBalancers(context.Context, *elbv2.DescribeLoadBalancersInput, ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	return &elbv2.DescribeLoadBalancersOutput{}, c.err
}
func (c *v2Stub) DescribeTargetGroups(context.Context, *elbv2.DescribeTargetGroupsInput, ...func(*elbv2.Options)) (*elbv2.DescribeTargetGroupsOutput, error) {
	return &elbv2.DescribeTargetGroupsOutput{}, c.err
}
func (c *v2Stub) DescribeTargetHealth(context.Context, *elbv2.DescribeTargetHealthInput, ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error) {
	return &elbv2.DescribeTargetHealthOutput{}, c.err
}

func successfulDependencies() dependencies {
	return dependencies{
		kubeClient: func(string, string) (scanner.KubeClient, error) { return &kubeStub{}, nil },
		awsConfig: func(context.Context, string, string) (aws.Config, error) {
			return aws.Config{Region: "us-east-1", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
			})}, nil
		},
		awsClients: func(aws.Config, string) scanner.RegionClients {
			return scanner.RegionClients{EC2: &ec2Stub{}, ELB: &clbStub{}, ELBV2: &v2Stub{}}
		},
	}
}

func TestScanStatusesAndExitCodes(t *testing.T) {
	private := "private-resource-https://private-cluster.invalid/account-123"
	denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: private}
	for _, tt := range []struct {
		name   string
		args   []string
		change func(*dependencies)
		code   int
		status string
		checks int
	}{
		{name: "complete empty scan", code: 0, status: "complete", checks: 5},
		{name: "Kubernetes only", args: []string{"--sources=kubernetes"}, change: func(d *dependencies) { d.awsConfig = nil; d.awsClients = nil }, code: 0, status: "complete", checks: 1},
		{name: "AWS only", args: []string{"--sources=aws"}, change: func(d *dependencies) { d.kubeClient = nil }, code: 0, status: "complete", checks: 4},
		{name: "unresolved discovery endpoint", args: []string{"--sources=aws"}, change: func(d *dependencies) {
			d.awsConfig = func(context.Context, string, string) (aws.Config, error) { return aws.Config{}, nil }
		}, code: 1, status: "failed", checks: 1},
		{name: "expired credentials", args: []string{"--sources=aws"}, change: func(d *dependencies) {
			d.awsConfig = func(context.Context, string, string) (aws.Config, error) {
				return aws.Config{Region: "us-east-1", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					return aws.Credentials{}, &smithy.GenericAPIError{Code: "ExpiredToken", Message: private}
				})}, nil
			}
		}, code: 1, status: "failed", checks: 4},
		{name: "all AWS APIs denied", args: []string{"--sources=aws"}, change: func(d *dependencies) {
			d.awsClients = func(aws.Config, string) scanner.RegionClients {
				return scanner.RegionClients{EC2: &ec2Stub{denied, denied}, ELB: &clbStub{denied}, ELBV2: &v2Stub{denied}}
			}
		}, code: 1, status: "failed", checks: 4},
		{name: "one AWS check denied", args: []string{"--sources=aws"}, change: func(d *dependencies) {
			d.awsClients = func(aws.Config, string) scanner.RegionClients {
				return scanner.RegionClients{EC2: &ec2Stub{errorIP: denied}, ELB: &clbStub{}, ELBV2: &v2Stub{}}
			}
		}, code: 3, status: "partial", checks: 4},
		{name: "Kubernetes configuration failure", change: func(d *dependencies) {
			d.kubeClient = func(string, string) (scanner.KubeClient, error) { return nil, errors.New(private) }
		}, code: 3, status: "partial", checks: 5},
		{name: "Kubernetes permission failure", args: []string{"--sources=kubernetes"}, change: func(d *dependencies) {
			d.kubeClient = func(string, string) (scanner.KubeClient, error) { return &kubeStub{err: denied}, nil }
		}, code: 1, status: "failed", checks: 1},
		{name: "all sources unavailable", change: func(d *dependencies) {
			d.kubeClient = func(string, string) (scanner.KubeClient, error) { return nil, errors.New(private) }
			d.awsConfig = func(context.Context, string, string) (aws.Config, error) { return aws.Config{}, errors.New(private) }
		}, code: 1, status: "failed", checks: 2},
		{name: "timeout with useful AWS results", args: []string{"--timeout=100ms"}, change: func(d *dependencies) {
			d.kubeClient = func(string, string) (scanner.KubeClient, error) { return &kubeStub{wait: true}, nil }
		}, code: 3, status: "partial", checks: 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := successfulDependencies()
			if tt.change != nil {
				tt.change(&deps)
			}
			var stdout, stderr bytes.Buffer
			args := append([]string{"--output=json"}, tt.args...)
			got := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), deps)
			if got != tt.code {
				t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", got, tt.code, stdout.String(), stderr.String())
			}
			var report struct {
				Schema int             `json:"schema_version"`
				Status string          `json:"status"`
				Checks []scanner.Check `json:"checks"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Schema != 1 || report.Status != tt.status || len(report.Checks) != tt.checks {
				t.Fatalf("report=%s", stdout.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), private) || strings.Contains(stdout.String(), "looks clean") {
				t.Fatal("provider details or false clean message leaked")
			}
		})
	}
}

func TestSourcesRunConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	kubeStarted := make(chan struct{})
	awsStarted := make(chan struct{})
	deps := successfulDependencies()
	deps.kubeClient = func(string, string) (scanner.KubeClient, error) {
		return &kubeStub{started: kubeStarted, release: awsStarted}, nil
	}
	original := deps.awsConfig
	deps.awsConfig = func(ctx context.Context, profile, region string) (aws.Config, error) {
		select {
		case <-kubeStarted:
		case <-ctx.Done():
			return aws.Config{}, ctx.Err()
		}
		close(awsStarted)
		return original(ctx, profile, region)
	}
	var stdout, stderr bytes.Buffer
	if code := runCommand(ctx, []string{"--output=json"}, testRuntime(&stdout, &stderr), deps); code != 0 {
		t.Fatalf("sources ran sequentially or failed: code=%d %s", code, stderr.String())
	}
}

func TestCancellationAndOutputFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps := successfulDependencies()
	deps.kubeClient = func(string, string) (scanner.KubeClient, error) { return &kubeStub{wait: true}, nil }
	var stdout, stderr bytes.Buffer
	if code := runCommand(ctx, []string{"--sources=kubernetes", "--output=json"}, testRuntime(&stdout, &stderr), deps); code != 130 {
		t.Fatalf("cancel exit=%d", code)
	}
	if code := runCommand(context.Background(), []string{"--sources=kubernetes"}, testRuntime(failedOutput{errors.New("broken pipe")}, &stderr), successfulDependencies()); code != 1 {
		t.Fatalf("write exit=%d", code)
	}
}

func TestDeadlineBoundsUnresponsiveAuthentication(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	deps := successfulDependencies()
	deps.kubeClient = func(string, string) (scanner.KubeClient, error) {
		<-release // Simulates a helper that does not observe context cancellation.
		return &kubeStub{}, nil
	}
	var stdout, stderr bytes.Buffer
	start := time.Now()
	code := runCommand(context.Background(), []string{"--sources=kubernetes", "--output=json", "--timeout=5ms"}, testRuntime(&stdout, &stderr), deps)
	if code != 1 || time.Since(start) > time.Second || !strings.Contains(stdout.String(), "timed out") {
		t.Fatalf("unbounded or incorrect timeout: exit=%d report=%s", code, stdout.String())
	}
}
