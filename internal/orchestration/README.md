# Orchestration

## Key Takeaways
- Centralises interactions with Nomad and Consul so API, CLI, and Mods workflows reuse the same submission, monitoring, and coordination helpers.
- Offers thin, testable abstractions for rendering HCL job specs, submitting jobs, streaming logs, and waiting for terminal or healthy states.
- Provides distributed coordination tools—Consul KV operations, health polling, and retry-aware transports—that higher-level packages rely on for resilient automation.

## Feature Highlights
- **Job Rendering & Submission**: Render Nomad job templates with context-aware substitutions, validate HCL, submit jobs, and wait for success or failure with timeout and retry controls.
- **Monitoring & Health Checks**: Wrap Consul health APIs and the SDK adapter so services, allocations, and orchestrated jobs expose consistent readiness signals.
- **KV Coordination**: Lightweight KV client that prefers JetStream buckets for locks, heartbeat data, and shared configuration, falling back to Consul only when JetStream is unavailable.
- **Streaming & Logging**: Attach to Nomad allocations, stream task logs, and surface structured events back to API/CLI callers for realtime feedback.
- **Retry Transport**: HTTP transport with exponential backoff, jitter, and error classification tailored for Nomad/Consul bursty workloads.

## Package Layout
- `render.go` – Template rendering pipeline for Nomad job specs; handles variable substitution, defaulting, and embedded template assets.
- `submit.go` – Public job submission helpers (`Submit`, `SubmitAndWaitHealthy`, `SubmitAndWaitTerminal`, `ValidateJob`, `DeregisterJob`).
- `monitor.go` / `monitor_sdk_adapter.go` – Generic monitoring interfaces and adapters for health probing, service readiness, and log streaming.
- `consul_health.go` / `monitor_endpoint_test.go` – Consul-specific health polling and endpoint verification utilities.
- `kv.go` – KV wrapper that defaults to Consul but can route to JetStream when enabled.
- `log_streamer.go` – Helpers that tail Nomad allocation logs with backoff and bounded buffers.
- `retry_transport.go` – Shared HTTP client wrapper with retry, timeout, and rate-limit handling tuned for control-plane calls.
- `templates_embed.go` – Embedded Nomad job templates (planner, reducer, builders) and related test fixtures.
- `osenv.go` – Centralised environment variable readers for orchestration defaults (Nomad/Consul addresses, tokens, retry settings).

## Usage Notes
- Prefer calling `SubmitAndWaitTerminal` for batch-style jobs (e.g., Mods planner/reducer) and `SubmitAndWaitHealthy` for long-running services; both emit controller events while polling.
- Use the KV helper when implementing distributed locks or leader election; it keeps API-compatible with the rest of the codebase.
- The retry transport is shared across API handlers—reuse it instead of instantiating custom HTTP clients to benefit from standard backoff and telemetry.
- When adding new Nomad templates, update `templates_embed.go` and include matching tests to keep render and submission code covered.

## Related Docs
- `internal/build/README.md` – Describes how build triggers use orchestration utilities for builder submission.
- `internal/mods/README.md` – Shows Mods runners wiring planner/reducer flows through this package.
- `platform/README.md` – Cluster-level configuration that these helpers assume.
