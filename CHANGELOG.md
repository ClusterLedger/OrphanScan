# Changelog

## 0.1.0 — unreleased beta

- Require Go 1.26.10 or newer to include the crypto/x509 security fix for GO-2026-6809.
- Present compact terminal reports with source summaries, unavailable/partial estimates, grouped recovery guidance, and optional `--verbose` coverage and pricing details.
- Match region-discovery guidance to typed failures, and avoid repeating reported scan issues as warnings and final errors.
- Record completion per source, region, and resource check; report partial coverage and meaningful nonzero exits.
- Restrict load balancer waste candidates to empty backends; paginate target groups and keep unhealthy registered backends in an unpriced review section.
- Protect PVCs referenced by any existing Pod phase.
- Sanitize provider diagnostics and mask arbitrary names, namespaces, IPs, and IDs consistently.
- Introduce JSON schema 1 with separate AWS and Kubernetes reference estimates; remove the unversioned combined total.
- Correct baseline load balancer prices and account for GWLB Availability Zones.
- Add source selection, Kubernetes context selection, standard KUBECONFIG loading, regional deduplication, scan deadlines, and bounded concurrency.
- Discover enabled AWS regions automatically when no region is configured, with `--all-regions` to override a configured scan region; add read-only `ec2:DescribeRegions` permission.
- Upgrade dependencies to patched versions and gate releases on vulnerability checks, lint, race tests, installer tests, cross-builds, and native platform smoke tests.
- Validate configured release telemetry settings before embedding them in binaries.
- Verify installation checksums, support pinned versions and custom installation directories, and preserve existing installations on validation failures.
- Reject installer destinations that are directories or symlinks to directories instead of reporting a successful installation.
- Include a synthetic credential-free demo, permission examples, report documentation, and release verification steps.

This beta identifies candidates and estimates reference costs. Cleanup belongs to the separate OrphanSweep product.
