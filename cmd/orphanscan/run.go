package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/awsauth"
	"github.com/ClusterLedger/OrphanScan/pkg/kubeauth"
	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

// run orchestrates authentication, scanning, and output rendering.
func run(ctx context.Context, cfg Config) error {
	if cfg.OutputFmt != "text" && cfg.OutputFmt != "json" {
		return fmt.Errorf("--output must be text or json")
	}
	usage := newUsageReporter(cfg)
	var report output.WasteReport

	k8sClient, k8sOK := initK8s(cfg.Kubeconfig)
	regions, awsCfg, awsOK := initAWS(ctx, cfg.AWSProfile, cfg.Regions)

	if !k8sOK && !awsOK {
		return fmt.Errorf("both Kubernetes and AWS are unavailable — nothing to scan")
	}

	if k8sOK {
		pvcs, err := scanner.ScanOrphanedPVCs(ctx, k8sClient)
		if err != nil {
			output.Warn("K8s PVC scan failed: %v", err)
		} else {
			report.PVCs = pvcs
		}
	}

	if awsOK && len(regions) > 0 {
		awsResult := scanner.ScanAWS(ctx, awsCfg, regions, nil, output.Warn)
		report.Volumes = awsResult.Volumes
		report.EIPs = awsResult.EIPs
		report.LBs = awsResult.LBs
	}

	return finishScan(ctx, os.Stdout, cfg.OutputFmt, report, usage)
}

func initK8s(kubeconfig string) (*kubeauth.Client, bool) {
	client, err := kubeauth.NewClient(kubeconfig)
	if err != nil {
		output.Warn("Kubernetes unavailable: %v", err)
		return nil, false
	}
	return client, true
}

func initAWS(ctx context.Context, profile string, regions []string) ([]string, aws.Config, bool) {
	cfg, err := awsauth.LoadConfig(ctx, profile, firstOrEmpty(regions))
	if err != nil {
		output.Warn("AWS unavailable: %v", err)
		return regions, aws.Config{}, false
	}
	if len(regions) == 0 && cfg.Region != "" {
		regions = []string{cfg.Region}
	}
	return regions, cfg, true
}

func finishScan(ctx context.Context, w io.Writer, format string, report output.WasteReport, usage *telemetry.Reporter) error {
	var err error
	switch format {
	case "json":
		err = output.PrintJSON(w, report)
	default:
		err = output.PrintText(w, report)
	}
	if err != nil {
		return err
	}
	if err := usage.Report(ctx, report.EstimatedMonthlyWaste()); err != nil {
		output.Warn("Usage reporting failed: %v", err)
	}
	return nil
}

// firstOrEmpty returns the first element or "" if the slice is empty.
func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
