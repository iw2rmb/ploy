# Mods Timeout Guards & Resiliency

## Why

- Long-running Mods can hang workstations and CI nodes when they lack timeouts or retry policies, blocking contributors during the RED → GREEN cadence.
- SHIFT and workflow-runner failures should produce actionable feedback; opaque hangs erode confidence in the CLI and the broader testing story.

## Required Changes

- Introduce timeout guards and bounded retry logic around long-running Mods within the workflow runner, defaulting to values aligned with `docs/v2/testing.md`.
- Ensure the CLI surfaces descriptive status updates (progress bars, countdowns, cancellation guidance) when operations approach or exceed thresholds.
- Capture timeout and retry telemetry so failures are observable in logs and metrics dashboards, including structured error codes for automation.
- Wire the new guards into both local harness execution and CI pipelines so behavior is consistent across environments.

## Definition of Done

- All long-running Mods invoked during local or CI test runs respect documented timeout values and bail out with actionable guidance instead of hanging.
- RETRY attempts are capped and logged, with final failures exiting non-zero and pointing developers to remediation docs.
- Observability dashboards (or log aggregation) expose timeout, retry, and cancellation metrics so the team can monitor trends.

## Tests

- Integration tests that run representative Mods end-to-end under controlled timeouts, asserting the workflow runner exits cleanly with informative messaging.
- Regression suites simulating SHIFT failures to verify retries trigger and eventual failures surface descriptive CLI output.
- Unit tests for new timeout/retry helpers ensuring edge cases (instant failures, repeated partial progress) are handled deterministically.
