# Releasing the public beta

[Back to README](../README.md) · [Contributor checks](../CONTRIBUTING.md#build-and-test) · [Installation](installation.md)

The first planned tag is **v0.1.0**, published as a non-prerelease with beta maturity stated in the README. This makes it discoverable by GitHub's latest-release endpoint and the default installer. Tagging or publishing is a maintainer action after review.

## Local and CI checks

1. Review the diff, changelog, README commands, JSON schema, and permissions examples.
2. Run the [contributor development checks](../CONTRIBUTING.md#build-and-test), including the binary smoke test. It clears real AWS configuration, uses local AWS/Kubernetes fixtures, and routes unexpected external requests to a local fixture proxy.
3. Validate with **GoReleaser v2.19.0**: `goreleaser check`, then `goreleaser release --snapshot --clean`. Snapshot builds produce archives for Linux/macOS/Windows on amd64/arm64 and `checksums.txt`; they do not publish. Run `python3 scripts/verify_release.py dist` to verify all archives, smoke-test the native binary, and install the staged archive through an offline download shim. This is also a CI gate.
4. Inspect archive contents, verify each checksum, and run the native Linux archive's `--version`, `--help`, and smoke tests. GitHub CI also performs native platform tests on Linux, macOS, and Windows. Cross-builds do not prove native ARM execution; record any native architecture validation still outstanding.
5. Confirm CI is green for the exact reviewed commit. The release workflow repeats the reusable test and lint gates before publication.

## Live verification

Use an explicitly selected test AWS identity and Kubernetes context that you are authorized to inspect, with the documented read-only permissions. Record the CLI version, source/region coverage, status, approximate runtime, and whether findings match an independent inventory. Keep identifiers out of shared notes.

- Check a complete scan, a known empty-backend balancer, and a PVC referenced by a Pending Pod.
- Confirm a missing permission produces a failed check and a partial or failed overall result, with nonzero exit.
- Confirm AWS and PVC estimates remain separate even when the PVC uses an EBS volume.
- Verify configured single-region coverage and `--all-regions` coverage independently. Check automatic discovery with an isolated profile/environment containing no region; it starts in commercial AWS at `us-east-1`. Discovery failures must not fall back to narrower coverage or count as completed resource checks.
- Refresh test credentials as needed; never add credentials or a kubeconfig to the repository.

Automated fixture tests do not substitute for this live verification.

## Telemetry and support setup

If publishing a telemetry-configured binary, complete [the PostHog release verification](telemetry.md#dashboard-and-release-verification), including opt-in/out, provider IP settings, private dashboard access, and payload inspection. Without repository reporting variables, binaries stay unconfigured and do not prompt or upload. A GoReleaser preflight validates configured hosts and public-token prefixes before any reporting settings are embedded. Invalid configuration stops the build without echoing its values.

Confirm GitHub private vulnerability reporting remains enabled and issue reporting is enabled. Verify `security@clusterledger.io` is monitored if email reports are supported. Provider access, IP handling, and mailbox monitoring cannot be established by a local build.

## Publication and clean installation

After explicit approval of the reviewed changes:

1. Commit and push the reviewed commit, then tag and push `v0.1.0`.
2. Wait for the gated release workflow to publish all six archives and `checksums.txt`.
3. Install into a fresh temporary `INSTALL_DIR` using the [pinned version command](installation.md#linux-and-macos). Also test the default latest-release path and a release-pinned `go install` with a temporary `GOBIN`.
4. Check `--version`, `--help`, JSON completion status, checksums, and expected exit codes from the downloaded build. Do not use a snapshot's development version as evidence for a published tag.
5. Recheck links and mark the changelog released with the publication date.

## Sharing

Use the [overview diagram](images/orphanscan-overview.png) for presentations and social posts. Its [editable SVG](images/orphanscan-overview.svg) is the source for both formats. After editing the SVG, regenerate the PNG with ImageMagick from the repository root:

```sh
magick -background white docs/images/orphanscan-overview.svg -strip PNG24:docs/images/orphanscan-overview.png
```

Generate the credential-free demo with `go run ./scripts/demo` and `go run ./scripts/demo --output=json`. The committed examples are synthetic and labelled as such. Capture a short terminal demo from that command for social posts; do not substitute real resource identifiers.

Suggested announcement:

> I built OrphanScan, an open-source, read-only CLI for finding unattached AWS resources and Kubernetes PVC candidates. It reports baseline monthly cost estimates, masks resource identifiers, and distinguishes completed scans from incomplete coverage. It is in beta; findings need verification before any cleanup. I would appreciate feedback on setup, false positives, and the report format.

Link the repository, release, sample report, and known limitations. Present estimates as estimates; avoid claims of measured savings from synthetic examples. Follow each community's current posting and self-promotion rules before submitting. Include the exact version, sources, regions, completion status, and sanitized warnings when collecting bug reports.
