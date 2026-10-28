package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/ClusterLedger/OrphanScan/pkg/output"
)

var (
	version               = "dev"
	telemetryProjectToken string
	telemetryHost         string
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runCommand(ctx, os.Args[1:], systemRuntime(), defaultDependencies())
	stop()
	os.Exit(code)
}

func buildVersion() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return strings.TrimPrefix(info.Main.Version, "v")
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return "dev (" + setting.Value[:min(12, len(setting.Value))] + ")"
			}
		}
	}
	return "dev"
}

func runCommand(ctx context.Context, args []string, runtime commandRuntime, deps dependencies) int {
	cfg, err := parseFlags(args, runtime.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}

	if err != nil {
		_, _ = fmt.Fprintf(runtime.stderr, "[ERROR] %v\n", err)
		return 2
	}

	if cfg.Version {
		if _, err := fmt.Fprintf(runtime.stdout, "OrphanScan %s\n", buildVersion()); err != nil {
			return 1
		}
		return 0
	}

	if err := runWithDependencies(ctx, cfg, runtime, output.NewWarnFunc(runtime.stderr), deps); err != nil {
		var exit *exitError
		if errors.As(err, &exit) {
			if !exit.reported {
				_, _ = fmt.Fprintf(runtime.stderr, "[ERROR] %v\n", err)
			}
			return exit.code
		}
		_, _ = fmt.Fprintf(runtime.stderr, "[ERROR] %v\n", err)
		return 1
	}

	return 0
}
