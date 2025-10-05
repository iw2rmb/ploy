# Health Checks & Consul Cleanup

> **Status (2025-09-25):** Completed — JetStream health probes back
> `/health`/`/ready`, Consul KV paths removed, and documentation refreshed.

## What to Achieve

Update platform health checks to cover JetStream readiness, deprecate Consul KV
dependencies, and remove legacy Consul code once migration tasks complete.

## Why It Matters

Ensuring observability and removing dead code prevents operators from relying on
stale paths and confirms the platform is fully JetStream-backed.

## Where Changes Will Affect

- `api/health/checks.go` – add JetStream connectivity checks, retire Consul KV
  probes.
- Controllers/services still referencing Consul KV – final cleanup patches.
- Documentation (`docs/FEATURES.md`, `CHANGELOG.md`, relevant READMEs) –
  announce JetStream GA and Consul retirement for KV use cases.

## How to Implement

1. Introduce JetStream health probes (list buckets, ping stream) and surface
   results in `/health` and `/ready` endpoints.
2. Audit codebase for residual Consul KV usage, removing or replacing with
   JetStream equivalents.
3. Update release notes and change logs after confirming no runtime dependency
   remains on Consul KV.
4. Coordinate with ops to adjust monitoring dashboards/alerts to track JetStream
   metrics.
5. Refresh documentation immediately afterward, including migration summary in
   `CHANGELOG.md` and impacted READMEs.

## Expected Outcome

Health endpoints validate JetStream, Consul KV references are eliminated, and
documentation accurately reflects the new coordination layer.

## Tests

- Unit: Extend health checker tests to cover JetStream probes and ensure
  Consul-specific paths are gated.
- Integration: Run API readiness checks in CI with JetStream
  reachable/unreachable scenarios to confirm failure modes.
- E2E: Execute a platform smoke test post-cleanup to ensure deployments, Builds,
  Mods run without Consul KV present.
