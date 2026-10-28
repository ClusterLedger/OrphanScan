# Detection and pricing

[Back to README](../README.md) · [Usage](usage.md) · [Report format](report-format.md)

## What it detects

- **EBS volumes:** AWS reports `available`; the volume was created at least 24 hours ago. The age filter measures time since creation, not time since detachment.
- **Elastic IPs:** no associated instance or network interface. The estimate assumes an ordinary AWS-provided billable IPv4 address.
- **Classic load balancers:** no registered instances.
- **ALB / NLB / GWLB:** no registered targets across all target groups and all pages. Registered unhealthy targets and disabled health checks are review findings, excluded from waste estimates.
- **Kubernetes PVCs:** Bound, created at least 24 hours ago, and not referenced by any existing Pod. Pending, Unknown, completed, and terminating Pod references also protect claims, as does ownership by an existing Pod for generic ephemeral storage.

## Investigate findings before cleanup

Every finding is a candidate. Reserved volumes, scaled-down StatefulSets, scheduled batch workloads, and recovery storage can be intentional. Empty ALBs can also serve redirects or fixed responses. The scanner does not inspect historical traffic, controller intent, retention policy, or deletion safety.

Cleanup is reserved for the separate OrphanSweep product. OrphanScan does not supply a deletion workflow.

## Cost estimates

Reference prices are from **us-east-1, checked 2026-10-26**, using a **720-hour month**:

- EBS and PVC storage: **$0.08/GiB/month**, using gp3 as a reference regardless of the actual storage backend or volume type.
- Elastic IP: **$3.60/month** at $0.005/hour.
- Classic load balancer: **$18.00/month** at $0.025/hour.
- ALB and NLB: **$16.20/month** at $0.0225/hour.
- GWLB: **$9.00 per enabled Availability Zone/month** at $0.0125/hour.

Sources: [EBS pricing](https://aws.amazon.com/ebs/pricing/), [public IPv4 pricing](https://aws.amazon.com/vpc/pricing/), and [load balancer pricing](https://aws.amazon.com/elasticloadbalancing/pricing/).

These are reference estimates, not regional or account-specific quotes. Capacity units, IOPS, throughput, data transfer, endpoints, load balancer IPv4 addresses, taxes, credits, and discounts are excluded. Address ownership and special IPv4 pool pricing are not assessed. No Pricing API is called. Money is rounded to USD cents per finding.

AWS and Kubernetes subtotals are never added because they can describe the same disk. Partial findings can underestimate cost, and repeated scans can describe the same resources. An estimate does not establish that a resource is safe to delete or that it equals your bill.

The [report format](report-format.md) describes how reference pricing assumptions, per-check coverage, and the two subtotals are recorded. Review coverage before interpreting a zero estimate.
