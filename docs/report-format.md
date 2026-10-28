# Report format, schema version 1

[Back to README](../README.md) · [Usage](usage.md) · [Detection and pricing](detection-and-pricing.md)

## Text output

Text is the default output format. The compact report starts with overall status and a summary for each source, followed by findings grouped by source, region, and resource. Monthly baseline estimates are separate for AWS and Kubernetes. Load balancers needing additional investigation appear under `Unpriced reviews` and are excluded from estimates. See the synthetic [text example](../examples/report.txt).

Source status and estimates describe the recorded coverage:

- **Complete:** all recorded resource checks for that source completed. A verified empty result can show `$0.00/month`.
- **Partial:** some usable results exist, but coverage is incomplete. Amounts are marked `available results only` and may underestimate cost, including when the available subtotal is zero.
- **Not scanned / Failed:** configuration failed or no resource results were usable. The estimate is `Unavailable` rather than a misleading zero.
- **Unknown:** check metadata was not supplied, as can happen with Go API callers constructing finding-only reports. Existing findings and their estimates are retained with `coverage unknown`; they do not establish a complete scan.

`Action needed` groups identical diagnostics per source, with a sanitized explanation, recovery guidance, and affected check/region counts. Text scan failures are printed once in the report on stdout. Expected partial and failed outcomes do not add duplicate warnings or a generic final error to stderr. Argument-validation and report-write failures still produce stderr diagnostics.

Use `--verbose` to include every check with readable resource labels, regions, and reference pricing. Detailed check rows refer to coverage; diagnostic explanations remain in the grouped issue section. The same plain-text layout works in terminals, redirected files, and CI without ANSI styling. Identifiers remain masked and provider diagnostics sanitized.

An interrupted command displays `Status: Interrupted` in text output and retains completed findings. This display label is separate from the coverage status described below: JSON still reports `complete`, `partial`, or `failed` according to the recorded checks, and an interrupted command exits with code 130.

When the report's coverage status is not `failed` and its preferred monthly estimate exceeds **$49.00**, text output ends with an OrphanSweep coming-soon notice and the ClusterLedger link. The preferred estimate selects the AWS subtotal when an AWS check has usable results (including a successful zero estimate) or AWS findings exist; otherwise it selects the Kubernetes subtotal. The two subtotals are never added. This notice does not provide a cleanup operation and is omitted from JSON.

An authentication failure during discovery can produce:

```text
OrphanScan
Status: Partial

Sources
  AWS          Not scanned
  Kubernetes   Complete - no PVC candidates found

No priced candidates found in available results.

Monthly baseline estimates
  AWS          Unavailable
  Kubernetes   $0.00/month

Action needed
  AWS: Authentication failed while discovering regions.
    Refresh or correct your AWS credentials, then retry.

Estimates are separate; storage may overlap.
Baseline estimates. Review candidates before cleanup.
Use --verbose for check and pricing details.
```

The permission-specific `ec2:DescribeRegions` / `--regions` remedy is shown when discovery is denied. Authentication, expiry, connectivity, and timeout failures receive guidance for their own cause.

## Completion status and exit codes

Overall status is `complete`, `partial`, or `failed`. Completion describes requested coverage, independently of whether candidates were found. No attempted checks or no usable results means `failed`.

- **0:** every requested check completed, including a complete scan with no candidates.
- **1:** no usable results, a fatal failure, or a report write failure.
- **2:** invalid arguments.
- **3:** usable partial results, with some requested checks incomplete.
- **130:** interrupted scan.

A partial report is printed before its nonzero exit. Preserve that file even if your shell or CI treats the command as failed. A wholly failed report is emitted with exit code 1. Output failures can prevent a complete JSON document and also return 1.

## JSON schema

JSON output always includes `schema_version`, `status`, `checks`, `currency`, `pricing`, `estimated_aws_monthly_cost_usd`, and `estimated_kubernetes_monthly_cost_usd`. Finding arrays are omitted when empty. See the generated [example](../examples/report.json).

`schema_version` is `1`. `status` records the overall completion status described above.

The compact/verbose text modes do not change JSON fields or numeric estimates. Interpret JSON estimates alongside `status` and `checks`; a failed source can have a numeric zero that does not establish empty coverage. `--verbose` has no effect on JSON. Grouped human-readable scan issues go to stderr while stdout remains a JSON document. The wording of sanitized error strings can change, and recovery guidance can contain escaped newlines.

Each check records:

- `source`: `aws` or `kubernetes`.
- `region`: AWS region when known; omitted for Kubernetes or an unresolved AWS configuration.
- `resource`: `ebs_volumes`, `elastic_ips`, `classic_load_balancers`, `elbv2_load_balancers`, `persistent_volume_claims`, or `configuration` when AWS initialization or region discovery fails before scan coverage is resolved.
- `status`: `complete`, `partial`, or `failed`. Partial load balancer checks retain findings for the balancers successfully evaluated.
- `error`: a sanitized diagnostic, omitted on complete checks. It does not contain provider error text, account identifiers, resource names, or cluster URLs.

Only requested sources appear. An unselected source is not a failure. A selected source with unavailable credentials, configuration, permissions, or failed region discovery is a failure.

See [AWS region selection](usage.md#aws-region-selection) for explicit coverage, automatic discovery, and discovery failure behavior. Successful discovery does not count as a completed resource check.

Finding arrays are `unattached_ebs_volumes`, `unassociated_eips`, `idle_load_balancers`, and `unattached_pvcs`. The legacy array names are retained, but load balancers qualify only when they have no registered backends. `load_balancers_for_review` contains registered unhealthy/unknown backends, disabled health checks, or unsupported load balancer types. Review findings have no `monthly_cost` and are excluded from both subtotals.

All identifying names, namespaces, and IPs are masked. Standard AWS IDs retain only a recognized type prefix and the last four hex characters. There is no option to output full identifiers.

Costs are USD reference estimates, with the region, date, and 720-hour month recorded in `pricing`. `size_gb` retains its earlier name and represents GiB. Monetary values are rounded to cents per finding, then summed within each source. AWS and Kubernetes estimates are separate because they may describe the same storage. A zero value does not imply complete coverage; inspect the checks.

See [baseline pricing assumptions](detection-and-pricing.md#cost-estimates) for the reference rates and exclusions.

## Migration from unversioned reports

This version replaces the earlier unversioned combined `total_monthly_waste` field. Update scripts to select an explicitly named source subtotal. There is no combined replacement field.
