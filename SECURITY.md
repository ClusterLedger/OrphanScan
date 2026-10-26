# Security Policy

## Scope

OrphanScan is a read-only scanning tool. It does not modify, delete, or create any cloud or Kubernetes resources. It does not persist cloud credentials or detailed scan findings. Optional usage reporting can retain a consent preference locally and send a minimal cost summary to the maintainers' PostHog project after explicit opt-in.

## Supported versions

Security fixes are applied to the latest release only.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| Older releases | No |

## Reporting a vulnerability

If you discover a security issue, please report it privately. **Do not open a public GitHub issue.**

Email: **security@clusterledger.io**

Alternatively, use [GitHub private vulnerability reporting](https://github.com/ClusterLedger/OrphanScan/security/advisories/new) if enabled on this repository.

Please include:
- A description of the vulnerability and its potential impact.
- Steps to reproduce, if possible.
- The version of OrphanScan affected.

We will acknowledge receipt within 3 business days and aim to provide a fix or mitigation within 14 days for confirmed issues.

## Credential handling

OrphanScan reads credentials through the standard AWS SDK default chain and Kubernetes kubeconfig. It never writes credentials to disk, logs, or stdout. Resource identifiers in scan output are obfuscated by default.

## Optional usage reporting

Reporting is off until the user agrees to the one-time invitation or sets `--telemetry=on`. One event represents one reported scan; the only application metric uploaded is `estimated_monthly_waste_usd`. Resource counts, resource and account identifiers, cluster names, namespaces, credentials, raw errors, scan status, and CLI version are excluded.

Each event has a new random identifier and timestamp required by the reporting protocol. No persistent installation ID or person profile is created. PostHog necessarily receives the network request and its source IP; maintainers must configure the project to discard IP data. This is minimal reporting, not a guarantee of complete anonymity.

The dashboard is private to authorized project members unless a maintainer enables public sharing. The local preference file contains only the consent boolean, with no findings, costs, or identifiers. There is no local reporting queue or offline history.

Use `--telemetry=off` to disable reporting persistently, or `DO_NOT_TRACK=1` to suppress it for one invocation. Suppressing reporting does not delete previously received events. See [telemetry setup and privacy details](docs/telemetry.md) for provider retention and maintainer configuration.
