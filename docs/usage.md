# Usage

[Back to README](../README.md) · [Installation](installation.md) · [Reports and exit codes](report-format.md) · [Read-only permissions](permissions/README.md)

## Defaults and requested coverage

```bash
orphanscan
```

By default both AWS and Kubernetes are requested, and output is a masked text report with separate baseline estimates. AWS scans one credential identity. Kubernetes uses standard `KUBECONFIG` merging or `~/.kube/config`, scanning one selected context across its namespaces.

The compact text report shows overall status, a summary for each source, findings, and grouped recovery guidance. A source with no usable results shows `Unavailable` for its estimate; partial estimates are labelled `available results only`. Use `--verbose` for individual resource checks, scanned regions, and reference pricing:

```bash
orphanscan --verbose
```

AWS uses the region resolved from the profile/environment. If no region is configured, it discovers enabled regions as described in [AWS region selection](#aws-region-selection). Kubernetes uses the current context unless overridden.

An unavailable source makes the scan incomplete, with a nonzero exit code. Use `--sources` to select the coverage you intend. See [completion status and exit codes](report-format.md#completion-status-and-exit-codes) before interpreting an empty or partial report.

## Choose sources

```bash
# AWS only, using the SDK's normal credential chain
orphanscan --sources=aws --regions us-east-1

# Kubernetes only, respecting KUBECONFIG and the current context
orphanscan --sources=kubernetes
```

The [permissions guide](permissions/README.md) includes copyable AWS IAM and Kubernetes RBAC examples. Credential providers can need their own authentication permissions.

## Credentials and Kubernetes contexts

```bash
# Both sources, multiple regions, a named AWS profile
orphanscan --sources=aws,kubernetes --aws-profile audit --regions us-east-1,eu-west-1

# Select a Kubernetes context or an explicit kubeconfig
orphanscan --sources=kubernetes --context audit
orphanscan --sources=kubernetes --kubeconfig /path/to/config
```

The standard AWS credential chain handles environment credentials, profiles, SSO, and role assumption. Refresh SSO or other credentials using your existing authentication tooling.

Kubeconfigs and profiles can execute external credential helpers. Use trusted configuration; helpers can produce their own diagnostics and update their own caches. OrphanScan does not add a credential store. See [credential handling](../SECURITY.md#credential-handling).

## AWS region selection

- With `--regions`, scan only the supplied regions. No region-discovery permission is needed.
- With `--all-regions`, discover and scan all regions enabled for the account in the configured AWS partition. This flag cannot be combined with a nonempty `--regions` list.
- With neither flag, scan the region resolved from the AWS profile/environment. If no region is configured, automatically discover all enabled regions.

```bash
# All enabled regions, even when a default region is configured
orphanscan --sources=aws --all-regions
```

Discovery uses the configured region as its starting endpoint. When none is configured, it starts in **us-east-1** for standard commercial AWS. For GovCloud, China, or another partition, configure a region from that partition through the AWS profile/environment and use `--all-regions` to scan its enabled regions. Discovery never enables disabled regions or scans across partitions.

Discovery requires `ec2:DescribeRegions`. Denied requests, empty results, and malformed responses fail AWS initialization; the CLI does not silently narrow the scan. Successful discovery does not count as a successful resource check and cannot make wholly failed resource scans appear usable. Its failures use a failed AWS `configuration` check. JSON and verbose text reports show the checks completed in each scanned region; compact text output summarizes source coverage.

## Export a report

```bash
# Inspect the exit code and the JSON completion status
orphanscan --sources=aws --regions us-east-1 --output=json > report.json
```

JSON includes schema version 1, overall completion status, individual checks, pricing assumptions, and separate AWS/Kubernetes estimates. See [report format](report-format.md) and the synthetic [text](../examples/report.txt) and [JSON](../examples/report.json) examples.

A partial report is printed before its nonzero exit. Preserve that file even if your shell or CI treats the command as failed. Review status and coverage before interpreting zero estimates.

## Scan time and concurrency

```bash
orphanscan --sources=aws --regions us-east-1,eu-west-1 --timeout=45s --concurrency=2
```

Discovery, authentication, and resource scans share the default two-minute timeout. Resource scans use at most four concurrent regions by default, with four resource checks per region. AWS and Kubernetes scans run concurrently. Larger inventories may need `--timeout=5m` or a narrower `--regions` selection.

## CLI reference

- `--sources`: comma-separated `aws` and/or `kubernetes`; default `aws,kubernetes`.
- `--regions`: comma-separated AWS regions; default the configured AWS region, or discover enabled regions if none is configured. Duplicates are scanned once.
- `--all-regions`: discover enabled regions even when an AWS region is configured; mutually exclusive with a nonempty `--regions` list.
- `--aws-profile`: named AWS profile; default SDK credential chain.
- `--kubeconfig`: explicit configuration file; otherwise standard `KUBECONFIG` merging or `~/.kube/config`.
- `--context`: Kubernetes context; default current context.
- `--output`: `text` or `json`; default `text`.
- `--verbose`: include individual checks and reference pricing in text output; default `false`. It has no effect on JSON output.
- `--timeout`: positive scan duration, including authentication; default `2m`.
- `--concurrency`: positive maximum concurrent AWS regions; default `4`. Each region runs four resource checks.
- `--telemetry`: save optional reporting preference, `on` or `off`.
- `--version`: print build or module version and exit.
- `--help`: show help and exit.

See [optional scan metrics](telemetry.md#consent-and-local-storage) for consent controls and preference storage.

## Troubleshooting

- **AWS authentication failed:** refresh or correct credentials using the selected profile's existing authentication tooling, then retry. Selecting regions or granting discovery permission does not fix invalid credentials.
- **AWS credentials expired:** refresh the credential session using your existing authentication tooling, then retry.
- **Region discovery failed:** grant `ec2:DescribeRegions`, or select coverage with `--regions`. A configured profile/environment region is scanned directly unless `--all-regions` is supplied.
- **Wrong AWS partition:** configure `AWS_REGION` / your AWS profile with a region from that partition before using `--all-regions`. Discovery without a configured region starts in commercial AWS.
- **Authentication failure:** refresh SSO or other credentials using your existing authentication tooling.
- **Permission denied:** compare your effective policy/RBAC with the [provided examples](permissions/README.md).
- **Wrong Kubernetes context:** inspect `kubectl config current-context`, then use `--context` or `--kubeconfig`.
- **Only one source available:** select it explicitly with `--sources`; otherwise expect a partial scan.
- **Slow or throttled scans:** reduce `--concurrency` or increase `--timeout`. AWS and Kubernetes scans run concurrently.

## Credential-free demo

From a clone with the required [Go version](installation.md#go), run:

```bash
go run ./scripts/demo
go run ./scripts/demo --verbose
go run ./scripts/demo --output=json
```

The demo uses the production report formatter to generate synthetic findings. It does not scan infrastructure or send telemetry, and its estimates are not measured customer savings. See the full [text example](../examples/report.txt) and [JSON example](../examples/report.json).
