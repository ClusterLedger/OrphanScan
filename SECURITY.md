# Security Policy

## Scope

OrphanScan is a read-only scanning tool. It does not modify, delete, or create any cloud or Kubernetes resources. It does not store credentials, tokens, or scan results beyond the lifetime of a single CLI invocation.

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
