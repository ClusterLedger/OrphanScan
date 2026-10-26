# OrphanScan

**Find forgotten cloud resources. Stop paying for what you don't use.**

[![Go Tests](https://github.com/ClusterLedger/OrphanScan/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterLedger/OrphanScan/actions/workflows/test.yml)
[![Lint](https://github.com/ClusterLedger/OrphanScan/actions/workflows/lint.yml/badge.svg)](https://github.com/ClusterLedger/OrphanScan/actions/workflows/lint.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

---

OrphanScan is a lightweight, **strictly read-only** CLI tool that scans your AWS account and Kubernetes cluster for idle or unattached resources and estimates the monthly waste. It runs from your terminal in seconds — no agents, no deployments, no write permissions required.

```text
=== OrphanScan: Waste Audit ===
Scanning local kubeconfig and AWS environment...

[Found] 4 Unattached EBS Volumes (us-east-1). Wasting: $160.00/mo
[Found] 2 Idle Load Balancers (eu-west-2). Wasting: $45.00/mo
[Found] 12 Unassociated Elastic IPs. Wasting: $43.20/mo
[Found] 3 Unattached K8s PVCs (default, staging). Wasting: $24.00/mo

Scan complete.
=========================================
Total Monthly Waste Detected: $272.20
=========================================
```

> Automated cleanup (OrphanSweep) coming soon.
> More from ClusterLedger: [github.com/ClusterLedger](https://github.com/ClusterLedger)

---

## Why OrphanScan?

Cloud waste is invisible. Teams spin up volumes, load balancers, and storage claims during development, then forget about them. These orphaned resources silently drain budgets — often hundreds of dollars per month.

OrphanScan makes this waste visible in one command.

- **Read-only** — zero risk to your infrastructure
- **Fast** — concurrent multi-region scanning, finishes in seconds
- **No setup** — uses your existing `kubectl` and AWS credentials
- **Portable** — single static binary, no runtime dependencies

---

## Install

No need to clone this repository.

### Install script (Linux / macOS)

```bash
curl -sSL https://raw.githubusercontent.com/ClusterLedger/OrphanScan/main/install.sh | sh
```

### With Go

```bash
go install github.com/ClusterLedger/OrphanScan/cmd/orphanscan@latest
```

### From a release archive

Download the binary for your platform from [GitHub Releases](https://github.com/ClusterLedger/OrphanScan/releases), extract it, and place `orphanscan` on your `PATH`.

---

## Quick start

```bash
# Scan AWS (us-east-1) + Kubernetes (current context)
orphanscan --regions us-east-1

# Scan multiple AWS regions
orphanscan --regions us-east-1,eu-west-1,ap-south-1

# Use a specific AWS profile
orphanscan --aws-profile audit --regions us-east-1

# Export JSON for scripting / dashboards
orphanscan --regions us-east-1 --output json > report.json

# Use a specific kubeconfig
orphanscan --kubeconfig /path/to/config --regions us-east-1
```

---

## What it detects

| Resource | Rule | Estimated cost |
| --- | --- | ---: |
| **EBS Volumes** | State is `available`, created >24h ago | $0.08 / GiB / month |
| **Elastic IPs** | Not associated with any instance or ENI | $3.60 / month |
| **Classic Load Balancers** | No instance is `InService` | $18.00 / month |
| **ALB / NLB** | No healthy target in any target group | $22.00 / $16.00 / month |
| **Kubernetes PVCs** | `Bound`, >24h old, not mounted by a running Pod | $0.08 / GiB / month |

> **Note:** Costs are baseline estimates hardcoded from us-east-1 gp3 pricing. They do not query the AWS Pricing API or vary by region. Findings are heuristics, not proof that a resource can be safely removed.

---

## CLI reference

| Flag | Description | Default |
| --- | --- | --- |
| `--kubeconfig` | Path to kubeconfig file | `~/.kube/config` |
| `--aws-profile` | Named AWS profile | SDK default chain |
| `--regions` | Comma-separated AWS regions | Region from AWS config |
| `--output` | Output format: `text` or `json` | `text` |
| `--telemetry` | Save optional scan metrics preference: `on` or `off` | Off until consent |

---

## Optional scan metrics

Sharing metrics helps us maintain OrphanScan. Reporting is **off by default**. In configured release builds, the first interactive text scan asks once whether you want to share the number of scans and estimated monthly waste through PostHog. Either answer is remembered.

```bash
# Enable reporting and remember the choice
orphanscan --telemetry=on --regions us-east-1

# Disable reporting and remember the choice
orphanscan --telemetry=off --regions us-east-1

# Disable reporting for this invocation without changing the saved choice
DO_NOT_TRACK=1 orphanscan --regions us-east-1
```

The dashboard counts one event per reported scan. Its only application metric is estimated monthly waste in USD. Resource counts, identifiers, credentials, scan status, and CLI version are not uploaded. Reports use a fresh random event ID rather than a persistent installation ID.

JSON output, redirected execution, and CI never prompt. Previously enabled reporting still works in those modes unless `DO_NOT_TRACK=1` is set. Uploads have a two-second timeout and never fail a scan. Source builds without a configured PostHog destination neither upload nor prompt.

See [telemetry setup and privacy details](docs/telemetry.md) for the payload, preference location, data retention, and private dashboard setup.

---

## Required permissions

OrphanScan needs **read-only** access. It never writes, deletes, or modifies resources.

### AWS IAM actions

```text
ec2:DescribeVolumes
ec2:DescribeAddresses
elasticloadbalancing:DescribeLoadBalancers
elasticloadbalancing:DescribeInstanceHealth
elasticloadbalancing:DescribeTargetGroups
elasticloadbalancing:DescribeTargetHealth
```

See the AWS authorization references for [EC2](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ec2.html), [ELB Classic](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elb.html), and [ELBv2](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elbv2.html).

### Kubernetes RBAC

Cluster-wide `list` permission on `pods` and `persistentvolumeclaims`. See the [Kubernetes RBAC guide](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).

---

## How it works

```text
                  +-----------+
                  |  CLI args |
                  +-----+-----+
                        |
          +-------------+-------------+
          |                           |
  +-------v-------+         +--------v--------+
  |  K8s Scanner   |         |   AWS Scanner    |
  | (client-go)    |         | (aws-sdk-go-v2)  |
  |                |         |                  |
  | List PVCs      |         | EBS volumes      |
  | List Pods      |         | Elastic IPs      |
  | Cross-ref      |         | CLB / ALB / NLB  |
  +-------+--------+         | (per region,     |
          |                   |  concurrent)     |
          |                   +--------+---------+
          |                            |
          +-------------+--------------+
                        |
                 +------v------+
                 | Pricing     |
                 | Engine      |
                 +------+------+
                        |
                 +------v------+
                 | Output      |
                 | (text/json) |
                 +-------------+
```

1. Parse CLI flags and initialize auth for Kubernetes and AWS.
2. If one source is unavailable, warn and continue with the other.
3. Run scanners concurrently (one goroutine per AWS region).
4. Apply safety filters (24-hour age threshold, bound-phase check).
5. Calculate estimated monthly cost per finding.
6. Print a summary to stdout. Warnings go to stderr.

---

## Graceful degradation

OrphanScan is designed to be resilient:

- **AWS unavailable, K8s works** — scans K8s only, prints `[WARN]` to stderr.
- **K8s unavailable, AWS works** — scans AWS only, prints `[WARN]` to stderr.
- **Both unavailable** — exits with code 1.
- **One AWS region fails** — logs a warning, continues scanning other regions.

---

## Interpreting results

- A finding is a **candidate**, not a deletion order. Always verify before acting.
- Load balancers may temporarily show zero healthy targets during deployments.
- PVCs may belong to CronJobs or batch workloads that are not currently running.
- Review `[WARN]` lines on stderr before treating an empty report as a clean bill of health.
- Resource IDs are obfuscated in output to prevent accidental manual cleanup.

---

## Development

```bash
# Build
go build -o orphanscan ./cmd/orphanscan

# Test
go test ./... -v
go test -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Lint
golangci-lint run --timeout=5m ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Cutting a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

The [release workflow](.github/workflows/release.yml) builds cross-platform binaries and publishes them to [GitHub Releases](https://github.com/ClusterLedger/OrphanScan/releases).

---

## Project structure

```text
cmd/orphanscan/     CLI entry point and flag parsing
pkg/awsauth/        AWS credential and config loading
pkg/kubeauth/       Kubernetes client initialization
pkg/scanner/        K8s and AWS scanning logic
pkg/pricing/        Hardcoded cost estimation
pkg/output/         Text/JSON formatting and ID obfuscation
```

---

## License

Apache License 2.0. See [LICENSE](LICENSE).

---

<p align="center">
  Built by <a href="https://github.com/ClusterLedger">ClusterLedger</a>
</p>
