# SSH Transfer Slice C — Pin State & CLI Parity

- Status: Draft
- Owner: Codex
- Created: 2025-10-26
- Parent: `docs/design/cli-ssh-artifacts/README.md`

## Summary
Close the loop between control-plane artifact metadata and operator UX. This slice exposes pin health/replication metrics, retries stalled publishes, and moves the `ploy artifact *` CLI to the HTTP API so all interactions go through the new backend.

## Goals
- Augment the artifact/registry stores with pin-state fields (`queued`, `pinning`, `pinned`, `failed`), replication factors, and timestamps.
- Add a background reconciler that queries IPFS Cluster, updates pin state, and retries failures with backoff/alerting.
- Update the CLI `artifact` subcommands to call `/v1/artifacts` instead of talking directly to IPFS Cluster; ensure status/remove flows surface pin health and errors.
- Emit Prometheus metrics (`ploy_artifacts_pin_state`, `ploy_artifacts_retry_total`) plus structured logs for retries/failures.

## Non-Goals
- Building the base artifact or registry persistence layers (Slice A/B).
- Documentation refresh (Slice D).

## Plan
1. **Store Extensions** — Add pin-state fields and replication hints to the artifact documents. Provide helper methods to transition states atomically.
2. **Reconciler** — Run a periodic worker (configurable interval) that:
   - Lists artifacts needing pin updates.
   - Calls the IPFS Cluster API to fetch peer status.
   - Updates artifacts; schedules retries when state != desired.
3. **CLI Parity** — Refactor `internal/cli/artifact` to hit the HTTP API using the existing tunnel; deprecate direct IPFS env vars once parity is achieved.
4. **Metrics & Alerts** — Register Prometheus counters/gauges and add log entries when retries exceed thresholds so ops can alert on stuck pins.

## Testing
- Unit tests for the reconciler using a fake IPFS client to simulate success/failure transitions.
- CLI tests verifying JSON parsing, error propagation, and output formatting.
- Integration test that commits an artifact, simulates a pin failure, and ensures retry transitions the state once the fake IPFS client reports success.

## Risks / Follow-ups
- Need to coordinate retry cadence with IPFS Cluster limits (avoid thundering herd).
- CLI change requires migration guidance for operators who previously configured IPFS credentials locally; capture that in Slice D.
