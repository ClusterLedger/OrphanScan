package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"

	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

func TestDiscoveryAuthenticationReport(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		for _, verbose := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/verbose=%t", format, verbose), func(t *testing.T) {
				deps := successfulDependencies()
				deps.regionClient = func(aws.Config) scanner.RegionDiscoveryClient {
					return discoveryFunc(func(context.Context, *ec2.DescribeRegionsInput) (*ec2.DescribeRegionsOutput, error) {
						return nil, &smithy.GenericAPIError{Code: "AuthFailure", Message: "private-account at https://private-cluster.invalid"}
					})
				}
				deps.awsClients = func(aws.Config, string) scanner.RegionClients {
					t.Fatal("resource APIs ran after discovery failed")
					return scanner.RegionClients{}
				}
				args := []string{"--all-regions", "--output=" + format}
				if verbose {
					args = append(args, "--verbose")
				}
				var stdout, stderr bytes.Buffer
				code := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), deps)
				if code != 3 {
					t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
				}
				if format == "text" {
					if stderr.Len() != 0 || strings.Count(stdout.String(), "Authentication failed while discovering regions.") != 1 {
						t.Fatalf("duplicated error: stdout=%s stderr=%s", stdout.String(), stderr.String())
					}
					for _, want := range []string{"Status: Partial", "AWS          Not scanned", "Kubernetes   Complete - no PVC candidates found", "AWS          Unavailable", "Kubernetes   $0.00/month"} {
						if !strings.Contains(stdout.String(), want) {
							t.Fatalf("missing %q: %s", want, stdout.String())
						}
					}
					if strings.Contains(stdout.String(), "AWS          $0.00") {
						t.Fatal("AWS failure became a zero estimate")
					}
				} else {
					var result struct {
						Schema int             `json:"schema_version"`
						Status string          `json:"status"`
						AWS    float64         `json:"estimated_aws_monthly_cost_usd"`
						Checks []scanner.Check `json:"checks"`
					}
					if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Schema != 1 || result.Status != "partial" || result.AWS != 0 || len(result.Checks) != 2 {
						t.Fatalf("JSON changed: %s", stdout.String())
					}
					if strings.Count(stderr.String(), "Authentication failed while discovering regions.") != 1 {
						t.Fatal(stderr.String())
					}
				}
				combined := stdout.String() + stderr.String()
				for _, unwanted := range []string{"private-account", "private-cluster.invalid", "ec2:DescribeRegions", "[WARN]", "[ERROR]", "scan incomplete;"} {
					if strings.Contains(combined, unwanted) {
						t.Fatalf("unrelated/unsafe diagnostic %q: %s", unwanted, combined)
					}
				}
			})
		}
	}
}

func TestVerboseLeavesJSONUnchanged(t *testing.T) {
	var first, second, stderr bytes.Buffer
	for index, writer := range []*bytes.Buffer{&first, &second} {
		args := []string{"--output=json"}
		if index == 1 {
			args = append(args, "--verbose")
		}
		if code := runCommand(context.Background(), args, testRuntime(writer, &stderr), successfulDependencies()); code != 0 {
			t.Fatalf("exit=%d", code)
		}
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) || stderr.Len() != 0 {
		t.Fatalf("verbose changed JSON: %s %s %s", first.String(), second.String(), stderr.String())
	}
}

func TestReportedFailuresAndFatalErrors(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		deps := successfulDependencies()
		denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "private-error"}
		deps.awsClients = func(aws.Config, string) scanner.RegionClients {
			return scanner.RegionClients{EC2: &ec2Stub{denied, denied}, ELB: &clbStub{denied}, ELBV2: &v2Stub{denied}}
		}
		var stdout, stderr bytes.Buffer
		if code := runCommand(context.Background(), []string{"--sources=aws", "--output=" + format}, testRuntime(&stdout, &stderr), deps); code != 1 {
			t.Fatalf("exit=%d", code)
		}
		diagnostics := stdout.String()
		if format == "json" {
			diagnostics = stderr.String()
		}
		if strings.Count(diagnostics, "Permission denied.") != 1 || !strings.Contains(diagnostics, "Affects 4 checks across 1 region.") || strings.Contains(stderr.String(), "[ERROR]") {
			t.Fatalf("ungrouped failure: %s %s", stdout.String(), stderr.String())
		}
	}
	var stderr bytes.Buffer
	if code := runCommand(context.Background(), []string{"--sources=kubernetes"}, testRuntime(failedOutput{errors.New("closed stdout")}, &stderr), successfulDependencies()); code != 1 || !strings.Contains(stderr.String(), "could not write scan report") {
		t.Fatalf("lost fatal write error: exit=%d %s", code, stderr.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	stderr.Reset()
	if code := runCommand(ctx, []string{"--sources=kubernetes"}, testRuntime(&stdout, &stderr), successfulDependencies()); code != 130 || !strings.Contains(stdout.String(), "Status: Interrupted") || stderr.Len() != 0 {
		t.Fatalf("lost or duplicated interruption: exit=%d %s %s", code, stdout.String(), stderr.String())
	}
}
