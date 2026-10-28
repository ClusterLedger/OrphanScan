package main

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/awsauth"
	"github.com/ClusterLedger/OrphanScan/pkg/diagnostic"
	"github.com/ClusterLedger/OrphanScan/pkg/kubeauth"
	"github.com/ClusterLedger/OrphanScan/pkg/output"
	"github.com/ClusterLedger/OrphanScan/pkg/report"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

type exitError struct {
	code     int
	message  string
	cause    error
	reported bool
}

func (e *exitError) Error() string { return e.message }
func (e *exitError) Unwrap() error { return e.cause }

type dependencies struct {
	kubeClient   func(string, string) (scanner.KubeClient, error)
	awsConfig    func(context.Context, string, string) (aws.Config, error)
	awsClients   scanner.ClientFactory
	regionClient func(aws.Config) scanner.RegionDiscoveryClient
	usage        func(Config, commandRuntime) (*telemetry.Reporter, error)
}

type sourceResult struct {
	source string
	report report.WasteReport
}

func defaultDependencies() dependencies {
	return dependencies{
		kubeClient: func(path, contextName string) (scanner.KubeClient, error) {
			return kubeauth.NewClient(path, contextName)
		},
		awsConfig: awsauth.LoadConfig,
		regionClient: func(cfg aws.Config) scanner.RegionDiscoveryClient {
			return ec2.NewFromConfig(cfg)
		},
		usage: func(cfg Config, runtime commandRuntime) (*telemetry.Reporter, error) {
			return newUsageReporter(cfg, telemetryHost, telemetryProjectToken, runtime)
		},
	}
}

func runWithDependencies(ctx context.Context, cfg Config, runtime commandRuntime, warn scanner.WarnFunc, deps dependencies) error {
	cfg = cfg.withDefaults()
	if err := validateConfig(cfg); err != nil {
		return &exitError{code: 2, message: err.Error()}
	}
	var usage *telemetry.Reporter
	if deps.usage != nil {
		var err error
		usage, err = deps.usage(cfg, runtime)
		if err != nil && warn != nil {
			warn("Usage reporting disabled: %v", err)
		}
	}
	scanCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	result := scanSources(scanCtx, cfg, deps)
	if cfg.OutputFmt == "json" {
		// JSON consumers receive the unchanged wire format on stdout. Human
		// diagnostics are best-effort, like the other stderr warnings.
		_ = output.PrintIssues(runtime.stderr, result.Checks)
	}
	return finishScan(scanCtx, runtime.stdout, cfg.OutputFmt, result, usage, warn, output.TextOptions{Verbose: cfg.Verbose})
}

func scanSources(ctx context.Context, cfg Config, deps dependencies) report.WasteReport {
	results := make(chan sourceResult, len(cfg.Sources))
	for _, source := range cfg.Sources {
		go func() {
			if source == scanner.SourceKubernetes {
				results <- sourceResult{source, scanKubernetes(ctx, cfg, deps)}
			} else {
				results <- sourceResult{source, scanAWS(ctx, cfg, deps)}
			}
		}()
	}
	return collectSourceResults(ctx, cfg, results)
}

const cancellationGracePeriod = 100 * time.Millisecond

func collectSourceResults(ctx context.Context, cfg Config, results <-chan sourceResult) report.WasteReport {
	pending := make(map[string]bool, len(cfg.Sources))
	for _, source := range cfg.Sources {
		pending[source] = true
	}
	var combined report.WasteReport
	done := ctx.Done()
	var grace *time.Timer
	var graceDone <-chan time.Time
	for len(pending) > 0 {
		select {
		case result := <-results:
			delete(pending, result.source)
			mergeReport(&combined, result.report)
		case <-done:
			// Let context-aware SDK calls return their partial findings, but do
			// not wait indefinitely on a stuck external credential helper.
			done = nil
			grace = time.NewTimer(cancellationGracePeriod)
			graceDone = grace.C
		case <-graceDone:
			for source := range pending {
				mergeReport(&combined, failedSource(source, cfg.Regions, diagnostic.Message(ctx.Err())))
			}
			clear(pending)
		}
	}
	if grace != nil {
		grace.Stop()
	}
	return combined
}

func mergeReport(combined *report.WasteReport, partial report.WasteReport) {
	combined.HasAWS = combined.HasAWS || partial.HasAWS
	combined.HasK8s = combined.HasK8s || partial.HasK8s
	combined.Volumes = append(combined.Volumes, partial.Volumes...)
	combined.EIPs = append(combined.EIPs, partial.EIPs...)
	combined.LBs = append(combined.LBs, partial.LBs...)
	combined.LBReviews = append(combined.LBReviews, partial.LBReviews...)
	combined.PVCs = append(combined.PVCs, partial.PVCs...)
	combined.Checks = append(combined.Checks, partial.Checks...)
}

func failedSource(source string, regions []string, message string) report.WasteReport {
	report := report.WasteReport{HasAWS: source == scanner.SourceAWS, HasK8s: source == scanner.SourceKubernetes}
	if source == scanner.SourceKubernetes {
		report.Checks = []scanner.Check{{Source: source, Resource: "persistent_volume_claims", Status: scanner.StatusFailed, Error: message}}
		return report
	}
	if len(regions) == 0 {
		report.Checks = []scanner.Check{{Source: source, Resource: "configuration", Status: scanner.StatusFailed, Error: message}}
		return report
	}
	for _, region := range regions {
		for _, resource := range []string{"ebs_volumes", "elastic_ips", "classic_load_balancers", "elbv2_load_balancers"} {
			report.Checks = append(report.Checks, scanner.Check{Source: source, Region: region, Resource: resource, Status: scanner.StatusFailed, Error: message})
		}
	}
	return report
}

func scanKubernetes(ctx context.Context, cfg Config, deps dependencies) report.WasteReport {
	if err := ctx.Err(); err != nil {
		return failedSource(scanner.SourceKubernetes, nil, diagnostic.Message(err))
	}
	report := report.WasteReport{HasK8s: true}
	check := scanner.Check{Source: scanner.SourceKubernetes, Resource: "persistent_volume_claims", Status: scanner.StatusFailed}
	client, err := deps.kubeClient(cfg.Kubeconfig, cfg.KubeContext)
	if err != nil {
		check.Error = "Kubernetes configuration could not be loaded.\nCheck --kubeconfig, KUBECONFIG, and --context."
	} else if err := ctx.Err(); err != nil {
		check.Error = diagnostic.Message(err)
	} else {
		report.PVCs, err = scanner.ScanOrphanedPVCs(ctx, client)
		if err != nil {
			check.Error = diagnostic.Message(err)
		} else {
			check.Status = scanner.StatusComplete
		}
	}
	report.Checks = []scanner.Check{check}
	return report
}

func scanAWS(ctx context.Context, cfg Config, deps dependencies) report.WasteReport {
	if err := ctx.Err(); err != nil {
		return failedSource(scanner.SourceAWS, cfg.Regions, diagnostic.Message(err))
	}
	setup, err := loadAWSScanConfig(ctx, cfg, deps)
	if err != nil {
		message := "AWS configuration could not be loaded.\nCheck your profile and credential configuration."
		if ctx.Err() != nil {
			message = diagnostic.Message(ctx.Err())
		}
		return failedSource(scanner.SourceAWS, setup.regions, message)
	}
	if !scanner.IsValidRegionName(setup.config.Region) {
		return failedSource(scanner.SourceAWS, nil, "AWS configuration contains an invalid region.\nPass --regions or configure AWS_REGION.")
	}
	if setup.config.Credentials == nil {
		return failedSource(scanner.SourceAWS, setup.regions, "Credentials are unavailable.\nConfigure AWS credentials, or choose --sources=kubernetes.")
	}
	if _, err := setup.config.Credentials.Retrieve(ctx); err != nil {
		message := diagnostic.Message(err)
		if diagnostic.Classify(err) == diagnostic.Unknown {
			message = "AWS credentials could not be loaded.\nCheck your AWS profile and configured credential provider."
		}
		return failedSource(scanner.SourceAWS, setup.regions, message)
	}
	if setup.discover {
		setup.regions, err = scanner.DiscoverEnabledRegions(ctx, deps.regionClient(setup.config))
		if err != nil {
			return failedSource(scanner.SourceAWS, nil, diagnostic.AWSRegionDiscoveryMessage(err))
		}
	}
	result := scanner.ScanAWSWithConcurrency(ctx, setup.config, setup.regions, deps.awsClients, nil, cfg.Concurrency)
	return report.WasteReport{
		HasAWS: true, Volumes: result.Volumes, EIPs: result.EIPs,
		LBs: result.LBs, LBReviews: result.LBReviews, Checks: result.Checks,
	}
}

func finishScan(ctx context.Context, w io.Writer, format string, result report.WasteReport, usage *telemetry.Reporter, warn scanner.WarnFunc, options output.TextOptions) error {
	options.Interrupted = errors.Is(ctx.Err(), context.Canceled)
	if err := renderReport(w, format, result, options); err != nil {
		return &exitError{code: 1, message: "could not write scan report", cause: err}
	}
	status := result.Status()
	interruptionReported := format == "text" && options.Interrupted
	if errors.Is(ctx.Err(), context.Canceled) || status == scanner.StatusFailed {
		return scanCompletionError(ctx, status, interruptionReported)
	}
	reportUsage(ctx, result, usage, warn)
	return scanCompletionError(ctx, status, interruptionReported)
}

func renderReport(w io.Writer, format string, result report.WasteReport, options output.TextOptions) error {
	if format == "json" {
		return output.PrintJSON(w, result)
	}
	return output.PrintTextWithOptions(w, result, options)
}

func scanCompletionError(ctx context.Context, status string, interruptionReported bool) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return &exitError{code: 130, message: "scan interrupted", reported: interruptionReported}
	}
	switch status {
	case scanner.StatusFailed:
		return &exitError{code: 1, message: "no requested checks returned usable results", reported: true}
	case scanner.StatusPartial:
		return &exitError{code: 3, message: "scan incomplete; review failed checks in the report", reported: true}
	}
	return nil
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
