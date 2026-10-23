# Contributing to OrphanScan

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
go test ./... -v
go test -count=1 -coverprofile=coverage.out ./...
```

All scanner functions accept interfaces so they can be tested with mocks. New scanner logic should follow this pattern.

## Code style

- Run `gofmt` before committing. CI will reject unformatted code.
- Run `golangci-lint run --timeout=5m ./...` to catch common issues.
- Keep functions small and focused. Extract helpers rather than writing long functions.
- Prefer table-driven tests with `t.Run` subtests.

## Pull requests

1. Ensure all tests pass and there are no lint warnings.
2. Update the README if your change affects user-facing behavior or flags.
3. Open a pull request against `main` with a clear description of what changed and why.
4. One approval is required before merging.

## Reporting bugs

Open a [GitHub issue](https://github.com/ClusterLedger/OrphanScan/issues) with:
- The command you ran and the flags you used.
- Expected vs. actual behavior.
- OrphanScan version (`orphanscan --help` or the release tag).
- Relevant `[WARN]` output from stderr.

## Code of conduct

Be respectful and constructive. We are building something useful together.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
