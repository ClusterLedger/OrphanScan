package main

import (
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/ClusterLedger/OrphanScan/internal/telemetry"
	"github.com/ClusterLedger/OrphanScan/pkg/scanner"
)

const defaultTimeout = 2 * time.Minute

type Config struct {
	Kubeconfig  string
	KubeContext string
	AWSProfile  string
	Regions     []string
	AllRegions  bool
	Sources     []string
	OutputFmt   string
	Verbose     bool
	Version     bool
	Telemetry   string
	Timeout     time.Duration
	Concurrency int
}

func parseFlags(args []string, w io.Writer) (Config, error) {
	var cfg Config
	fs := flag.NewFlagSet("orphanscan", flag.ContinueOnError)
	fs.SetOutput(w)
	fs.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Kubeconfig path (default: KUBECONFIG or ~/.kube/config)")
	fs.StringVar(&cfg.KubeContext, "context", "", "Kubernetes context (default: current context)")
	fs.StringVar(&cfg.AWSProfile, "aws-profile", "", "AWS profile (default: SDK credential chain)")
	regions := fs.String("regions", "", "Comma-separated AWS regions (default: configured region, otherwise discover enabled regions)")
	fs.BoolVar(&cfg.AllRegions, "all-regions", false, "Discover all enabled AWS regions, overriding the configured scan region")
	sources := fs.String("sources", "aws,kubernetes", "Comma-separated sources: aws, kubernetes")
	fs.StringVar(&cfg.OutputFmt, "output", "text", "Output format: text or json")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Show individual checks and reference pricing in text output (no effect on JSON)")
	fs.BoolVar(&cfg.Version, "version", false, "Print version and exit")
	fs.StringVar(&cfg.Telemetry, "telemetry", "", "Save optional metrics preference: on or off")
	fs.DurationVar(&cfg.Timeout, "timeout", defaultTimeout, "Maximum scan duration, including authentication")
	fs.IntVar(&cfg.Concurrency, "concurrency", scanner.DefaultConcurrency, "Maximum concurrent AWS regions (four checks per region)")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("unexpected positional arguments; use --help")
	}
	cfg.Regions = splitUniqueCSV(*regions)
	cfg.Sources = splitUniqueCSV(*sources)
	return cfg, validateConfig(cfg)
}

func validateConfig(cfg Config) error {
	if len(cfg.Sources) == 0 {
		return errors.New("--sources must include aws or kubernetes")
	}
	if cfg.OutputFmt != "text" && cfg.OutputFmt != "json" {
		return errors.New("--output must be text or json")
	}
	if err := telemetry.ValidateMode(cfg.Telemetry); err != nil {
		return err
	}
	if cfg.Timeout <= 0 {
		return errors.New("--timeout must be greater than zero")
	}
	if cfg.Concurrency < 1 {
		return errors.New("--concurrency must be at least one")
	}
	if cfg.AllRegions && len(cfg.Regions) > 0 {
		return errors.New("--all-regions cannot be combined with --regions")
	}

	for _, source := range cfg.Sources {
		if source != scanner.SourceAWS && source != scanner.SourceKubernetes {
			return errors.New("--sources accepts only aws and kubernetes")
		}
	}

	for _, region := range cfg.Regions {
		if !scanner.IsValidRegionName(region) {
			return errors.New("--regions contains an invalid AWS region name")
		}
	}

	return nil
}

func (cfg Config) withDefaults() Config {
	if cfg.OutputFmt == "" {
		cfg.OutputFmt = "text"
	}
	if cfg.Sources == nil {
		cfg.Sources = []string{scanner.SourceAWS, scanner.SourceKubernetes}
	} else {
		cfg.Sources = splitUniqueCSV(strings.Join(cfg.Sources, ","))
	}
	cfg.Regions = splitUniqueCSV(strings.Join(cfg.Regions, ","))
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = scanner.DefaultConcurrency
	}
	return cfg
}

// splitUniqueCSV trims and deduplicates a comma-separated list in first-seen order.
func splitUniqueCSV(s string) []string {
	var values []string
	seen := make(map[string]bool)
	for _, value := range strings.Split(s, ",") {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}
