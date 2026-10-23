package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all CLI options for the orphan-scan command.
type Config struct {
	Kubeconfig string
	AWSProfile string
	Regions    []string
	OutputFmt  string
	Version    bool
}

// ParseFlags reads CLI flags and returns a validated Config.
func ParseFlags() Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	defaultKubeconfig := filepath.Join(homeDir, ".kube", "config")

	kubeconfig := flag.String("kubeconfig", defaultKubeconfig, "Path to the kubeconfig file")
	awsProfile := flag.String("aws-profile", "", "AWS profile name (default: environment/config default)")
	regions := flag.String("regions", "", "Comma-separated list of AWS regions to scan")
	outputFmt := flag.String("output", "text", "Output format: text or json")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	return Config{
		Kubeconfig: *kubeconfig,
		AWSProfile: *awsProfile,
		Regions:    parseRegions(*regions),
		OutputFmt:  *outputFmt,
		Version:    *showVersion,
	}
}

// parseRegions splits a comma-separated string into a slice, trimming whitespace.
func parseRegions(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
