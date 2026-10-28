package main

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestConfigParsingAndDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"--sources=aws,aws,kubernetes", "--regions= us-east-1,eu-west-1,us-east-1 ", "--timeout=45s", "--concurrency=2", "--context=review", "--verbose"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Regions, []string{"us-east-1", "eu-west-1"}) || !reflect.DeepEqual(cfg.Sources, []string{"aws", "kubernetes"}) {
		t.Fatalf("config=%+v", cfg)
	}
	if cfg.Timeout != 45*time.Second || cfg.Concurrency != 2 || cfg.KubeContext != "review" || cfg.Kubeconfig != "" || !cfg.Verbose {
		t.Fatalf("config=%+v", cfg)
	}
	defaults, err := parseFlags(nil, io.Discard)
	if err != nil || defaults.Timeout != 2*time.Minute || defaults.Concurrency != 4 || defaults.Verbose {
		t.Fatalf("defaults=%+v err=%v", defaults, err)
	}
}

func TestInvalidFlagsDoNotAuthenticate(t *testing.T) {
	for _, args := range [][]string{
		{"unexpected", "--output=json"}, {"--sources="}, {"--sources=gcp"}, {"--output=yaml"},
		{"--concurrency=0"}, {"--timeout=0"}, {"--timeout=-1s"}, {"--telemetry=maybe"}, {"--regions=https://private-cluster.invalid"},
		{"--all-regions", "--regions=us-east-1"},
	} {
		var stdout, stderr bytes.Buffer
		// Nil dependency functions panic if parsing accidentally reaches auth.
		if got := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), dependencies{}); got != 2 {
			t.Errorf("%v: exit=%d stderr=%s", args, got, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("invalid args wrote a report: %s", stdout.String())
		}
	}
}

func TestAllRegionsFlag(t *testing.T) {
	for _, args := range [][]string{{"--all-regions"}, {"--all-regions", "--regions="}} {
		cfg, err := parseFlags(args, io.Discard)
		if err != nil || !cfg.AllRegions || len(cfg.Regions) != 0 {
			t.Fatalf("args=%v config=%+v err=%v", args, cfg, err)
		}
	}
}

func TestHelpAndVersionDoNotAuthenticate(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		var stdout, stderr bytes.Buffer
		if got := runCommand(context.Background(), args, testRuntime(&stdout, &stderr), dependencies{}); got != 0 {
			t.Fatalf("%v: exit=%d", args, got)
		}
	}
	original := version
	t.Cleanup(func() { version = original })
	version = "0.1.0"
	if buildVersion() != "0.1.0" {
		t.Fatal("linker version was not preferred")
	}
}
