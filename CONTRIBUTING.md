# Contributing to OrphanScan

[Back to README](README.md) · [Installation and prerequisites](docs/installation.md)

Thank you for considering a contribution to OrphanScan. This document explains how to get started.

## Scope

OrphanScan Phase 1 is **strictly read-only**. Contributions that add delete, modify, or state-change operations on any infrastructure will not be accepted. If you have ideas for cleanup automation, open an issue to discuss the direction first.

## Getting started

1. Fork the repository and clone your fork.
2. Create a branch from `main`:
   ```bash
   git checkout -b my-change
   ```
3. Make your changes, keeping commits focused and well-described.

## Build and test

```bash
go build -o orphanscan ./cmd/orphanscan
go test -race -count=1 -coverprofile=coverage.out ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go mod tidy -diff
sh scripts/test-install.sh
shellcheck install.sh scripts/*.sh
python3 scripts/check-docs.py
python3 scripts/smoke.py ./orphanscan
```

All scanner functions accept interfaces so they can be tested with mocks. New scanner logic should follow this pattern.

Regression tests use fake credentials and local API fixtures. Do not run tests against a contributor's real infrastructure. To verify a built executable on your platform, run `python3 scripts/smoke.py /path/to/orphanscan`.

The documentation check validates local Markdown links and heading anchors, then compares both committed report examples and the README sample with the production demo. It uses Python's standard library, Git, and Go. It does not scan infrastructure or contact external links; building the demo may download Go dependencies if they are not cached.

CI tests the native Linux, macOS, and Windows environments, builds all six OS/architecture combinations, and gates release publication on validation. Cross-building alone does not demonstrate native execution on every architecture. See [release verification](docs/releasing.md) for the full checklist.

Preserve per-check completion status, sanitized errors, permanently masked reports, and separate AWS/Kubernetes estimates. New detections must not treat API failures or missing pages as evidence that a resource is unused. Cleanup belongs to a separate product.

## Package responsibilities

- `pkg/scanner` detects candidates and records check completion through narrow provider interfaces.
- `pkg/report` owns the shared report model, cost aggregation, overall status, and the AWS-first selection of a single estimate without combining overlapping subtotals.
- `pkg/output` renders text and JSON, sorts copies of findings, and masks identifiers. Its `WasteReport` and `CostSummary` aliases preserve existing Go imports; new scan code should import `pkg/report`.
- `cmd/orphanscan` coordinates sources, cancellation, warnings, rendering, telemetry, and exit codes through separate helpers. Process streams and environment values are captured at the command boundary and passed explicitly.
- `internal/telemetry` handles consent, saved preferences, and delivery. The command reports usage after successful rendering. Footer visibility belongs to the text formatter.

Use `output.NewWarnFunc(writer)` for invocation-scoped warnings. `output.Warn`, `output.SetWarnWriter`, and `WasteReport.EstimatedMonthlyWaste()` remain available for compatibility. New code should use explicit writers and `PreferredMonthlyEstimate()` when a single estimate is needed, or `Costs()` for the separate subtotals.

## Code style

- Run `gofmt` before committing. CI will reject unformatted code.
- Run `golangci-lint run --timeout=5m ./...` to catch common issues.
- Keep functions small and focused. Extract helpers rather than writing long functions.
- Prefer table-driven tests with `t.Run` subtests.

## Pull requests

1. Ensure all tests pass and there are no lint warnings.
2. Update the relevant [user guide](README.md#documentation) if your change affects user-facing behavior or flags. Update the README overview and Quick start when those are affected.
3. Open a pull request against `main` with a clear description of what changed and why.
4. One approval is required before merging.

## Reporting bugs

Open a [GitHub issue](https://github.com/ClusterLedger/OrphanScan/issues) with:
- The command you ran and the flags you used.
- Expected vs. actual behavior.
- OrphanScan version (`orphanscan --version`).
- The exit code and a sanitized report, including the `Action needed` section for text output. Use `--verbose` when check and regional coverage details help explain the problem.
- Any stderr diagnostics. Text scan failures are included in the stdout report; JSON scans send grouped human-readable issues to stderr. Optional reporting warnings and fatal command errors also use stderr.

## Code of conduct

Be respectful and constructive. We are building something useful together.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
