# Optional scan metrics

[Back to README](../README.md) · [Usage](usage.md) · [Security policy](../SECURITY.md#optional-usage-reporting)

OrphanScan offers opt-in reporting to help maintain the project. It measures two things: the number of reported scans and estimated monthly waste per scan. The maintainers view those metrics in a private PostHog dashboard.

## Consent and local storage

Reporting is off by default. After a successfully rendered interactive text scan, a configured build asks:

```text
Help us maintain OrphanScan by sharing scan metrics via PostHog.
Includes the number of scans and estimated monthly waste.
Allow sharing for this and future scans? [y/N]
```

Only `y` or `yes` enables reporting. Enter, No, or any other answer disables it. Either choice is saved, so the invitation does not repeat. The CLI never asks during JSON output, redirected execution, CI (a nonempty `CI` variable), help, or version commands. A previously saved opt-in still permits reporting during noninteractive scans.

`--telemetry=on` and `--telemetry=off` change and save the preference before scanning, even if the scan subsequently fails. `DO_NOT_TRACK=1` suppresses reporting, prompting, and preference changes for the current invocation, taking precedence over either flag.

```bash
orphanscan --telemetry=on --sources=aws --regions us-east-1
orphanscan --telemetry=off --sources=aws --regions us-east-1
DO_NOT_TRACK=1 orphanscan --sources=aws --regions us-east-1
```

The preference is stored at `orphanscan/telemetry.json` inside Go's [`os.UserConfigDir()`](https://pkg.go.dev/os#UserConfigDir):

- Linux: `$XDG_CONFIG_HOME/orphanscan/telemetry.json`, or `~/.config/orphanscan/telemetry.json`.
- macOS: `~/Library/Application Support/orphanscan/telemetry.json`.
- Windows: `%AppData%\orphanscan\telemetry.json`.

Its complete contents are `{"enabled":true}` or `{"enabled":false}`. No installation ID, scan results, or costs are saved. Preference read/write failures disable reporting for that run and produce a warning. If a preference change cannot be saved, the previous saved preference remains; retry the change when the configuration directory is writable. `DO_NOT_TRACK=1` also works without a writable configuration directory.

## What is sent

For each opted-in, successfully rendered scan, the CLI makes one HTTPS POST to the configured PostHog Cloud ingestion host's `/i/v0/e/` endpoint. Complete empty scans and partial reports with usable results are eligible. Failed scans, interrupted/timed-out invocations, help, version, and output failures are excluded. Partial scans still return exit code 3 independently of reporting.

An example event is:

```json
{
  "api_key": "phc_PUBLIC_PROJECT_TOKEN",
  "event": "scan_completed",
  "distinct_id": "a fresh random identifier for this scan",
  "timestamp": "2026-10-23T12:00:00Z",
  "properties": {
    "estimated_monthly_waste_usd": 12.34,
    "$process_person_profile": false
  }
}
```

The dashboard counts events to calculate the number of reported scans. The sole application metric is `estimated_monthly_waste_usd`, using the same pricing calculation as text and JSON output. Because AWS and PVC findings may describe the same disk, the metric is the AWS subtotal if any AWS check returned usable results (including a successful empty AWS check); otherwise it is the Kubernetes subtotal. The two are never added. The report exposes both subtotals separately and records reference pricing assumptions; the telemetry payload remains limited to the one numeric metric. Resource counts, names, IDs, accounts, regions, namespaces, credentials, raw errors, scan status, and CLI version are not sent. There is no persistent identifier connecting separate scans, and PostHog is instructed not to create person profiles. See the [capture API documentation](https://posthog.com/docs/api/capture).

The provider still receives the HTTP request and its source IP. Disable IP capture in the PostHog project as described below. Dashboard access, provider processing, and retention are separate from the CLI's minimal payload; the reporting should not be described as completely anonymous.

Each request has a two-second timeout. Redirects and retries are disabled. Rejected requests, timeouts, and offline execution never fail a scan or change its output. Events are not queued to disk or resent later, so counts represent received reports rather than every invocation or every user.

Partial reports contain only findings available from working scan sources and may underestimate waste. Repeated scans can report the same resources. Adding their monthly estimates together does not measure distinct waste or actual savings.

## Maintainer setup: free private project

1. Create a PostHog account and a project on the **Free** plan without adding a payment card. Choose the project's cloud region. The current free allowance is 1 million analytics events per month with one-year data retention; additional events are dropped at the limit. Confirm the current [pricing and retention policy](https://posthog.com/pricing) when setting up the project.
2. Keep the project and dashboard restricted to authorized maintainers. Leave public sharing disabled; a public dashboard link permits outside access. See [dashboard sharing](https://posthog.com/docs/product-analytics/sharing).
3. In **Settings → Project → General**, configure IP data capture to **discard** client IP data. See [IP capture controls](https://posthog.com/docs/privacy/data-collection#ip-data-capture).
4. Copy the public **project token** beginning with `phc_`. Its capture permission is intended for distribution in client applications; it does not grant dashboard access. Never embed a personal (`phx_`) or secret (`phs_`) API key. See [PostHog key guidance](https://posthog.com/docs/privacy).
5. Set these GitHub repository **Actions variables** under **Settings → Secrets and variables → Actions → Variables**:
   - `ORPHANSCAN_POSTHOG_HOST`: `https://us.i.posthog.com` or `https://eu.i.posthog.com`, matching the project region.
   - `ORPHANSCAN_POSTHOG_PROJECT_TOKEN`: the public `phc_` project token.
6. The release workflow passes those variables to GoReleaser, which embeds them in release binaries. Builds without a token neither upload nor prompt. Ordinary `go install` builds do not inherit repository Actions variables and remain unconfigured.

The capture token is public and events are not authenticated as individual users. Treat the resulting metrics as approximate product feedback, not accounting or audit records.

For a local configured build, replace the public token placeholder and use the project's ingestion host:

```bash
go build -ldflags='-X main.telemetryHost=https://us.i.posthog.com -X main.telemetryProjectToken=phc_PUBLIC_PROJECT_TOKEN' -o orphanscan ./cmd/orphanscan
```

No user needs a PostHog account or token to use a configured release binary. Opting out prevents the extra network request.

## Dashboard and release verification

Create a private dashboard named **OrphanScan usage** with these insights:

1. **Reported scans:** total count of `scan_completed` events, shown daily and as a total for the selected period. Use event counts, not unique-person counts.
2. **Estimated monthly waste per scan:** average of the numeric `estimated_monthly_waste_usd` event property over the same period. Inspect individual events when a per-scan value is needed; do not present a sum across repeated scans as distinct savings.

Before releasing the configured build, run one explicit opt-in scan against infrastructure you are authorized to inspect and confirm that one event appears, its cost matches the report, person profiles are not created, IP capture is disabled, and the dashboard has no public sharing link. Disable reporting with `--telemetry=off` afterward if this was only a test. Automated tests use local fake services and never contact PostHog.

Disabling future reporting does not delete reports already received. Retention follows the PostHog project plan; maintainers should review access and retention in that project before publishing a configured release.
