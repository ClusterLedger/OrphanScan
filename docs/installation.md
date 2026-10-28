# Installation

[Back to README](../README.md) · [Usage](usage.md) · [Read-only permissions](permissions/README.md)

## Requirements

- Release binaries support Linux, macOS, and Windows on amd64 and arm64.
- Installing or building with Go requires **Go 1.26.9 or newer**.
- Scanning needs your existing AWS credentials and/or Kubernetes configuration, with the documented [read-only permissions](permissions/README.md). Select only the sources you intend to scan.
- Authentication may require the external helper already configured in your AWS profile or kubeconfig, such as the AWS CLI for an EKS exec plugin. Use trusted configuration; helpers can execute programs and update their own credential caches. See [credential handling](../SECURITY.md#credential-handling).

The installer and release archives require a published [GitHub Release](https://github.com/ClusterLedger/OrphanScan/releases). Before the first release is published, build from a clone using the [contributor build instructions](../CONTRIBUTING.md#build-and-test).

## Linux and macOS

```bash
curl -fsSL https://raw.githubusercontent.com/ClusterLedger/OrphanScan/main/install.sh | sh
```

The installer downloads the latest published non-prerelease, verifies the archive against its SHA-256 checksum, and installs atomically. It uses `/usr/local/bin` when writable, otherwise `~/.local/bin`. It never requests sudo. Ensure the chosen directory is on your `PATH`.

To pin a release and choose a directory:

```bash
curl -fsSL https://raw.githubusercontent.com/ClusterLedger/OrphanScan/main/install.sh \
  | VERSION=v0.1.0 INSTALL_DIR="$HOME/.local/bin" sh
```

The install script must be available in the public repository, and the selected binary version must have been published. The first planned release is `v0.1.0`.

## Go

Requires **Go 1.26.9 or newer**:

```bash
go install github.com/ClusterLedger/OrphanScan/cmd/orphanscan@latest
```

Use `@v0.1.0` to pin the first release after it is published. `--version` uses embedded release metadata or the module version for `go install` builds. Ensure Go's binary installation directory is on your `PATH`.

Ordinary `go install` builds do not inherit repository reporting settings and remain unconfigured for optional metrics. See [optional scan metrics](telemetry.md).

## Release archives and Windows

Download the matching archive and `checksums.txt` from [GitHub Releases](https://github.com/ClusterLedger/OrphanScan/releases). Verify the archive's SHA-256 hash before extracting, then place `orphanscan` or `orphanscan.exe` on your `PATH`.

On Windows, `Get-FileHash <archive.zip> -Algorithm SHA256` displays the hash to compare with `checksums.txt`. Use the archive matching your OS and architecture.

## Verify installation

```bash
orphanscan --version
orphanscan --help
```

These commands do not scan infrastructure. Continue with the [Quick start](../README.md#quick-start), or use the [credential-free demo](usage.md#credential-free-demo) from a clone.
