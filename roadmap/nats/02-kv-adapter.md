# JetStream KV Adapter

## What to Achieve

Introduce a JetStream-backed implementation of `internal/orchestration.KV` and
make it the default coordination backend, retaining Consul only as an automatic
fallback when JetStream is unavailable.

## Why It Matters

Abstracting JetStream behind the existing KV interface enables incremental
rollouts without rewriting every consumer, while enforcing revision-aware
semantics derived from the NATS Key-Value patterns.

## Where Changes Will Affect

- `internal/orchestration/kv.go` – new adapter, configuration wiring, automatic
  fallback.
- `api/server/` initialisers – dependency injection to choose between Consul and
  JetStream clients.
- `docs/internal/orchestration.md` or README – document the KV backend
  selection.

## How to Implement

1. Add a JetStream client factory that reads connection info and credentials
   from env (`NATS_ADDR`, etc.).
2. Implement KV methods (`Put`, `Get`, `Keys`, `Delete`) using JetStream
   buckets, mapping CAS failures to explicit errors.
3. Extend configuration structs to expose connection settings (URL, credentials)
   and deprecate the feature flag.
4. Update unit tests with JetStream-backed fakes or an embedded server,
   borrowing patterns from the NATS Key-Value example.
5. Ensure logs/metrics differentiate backend type for observability.
6. Update documentation (`internal/orchestration/README.md`, `docs/FEATURES.md`)
   after completion.

## Expected Outcome

An interchangeable KV layer that can target JetStream without code churn for
downstream packages, guarded by an opt-in flag.

## Tests

- Unit: New tests covering JetStream KV operations and CAS error handling.
- Integration: Spin an ephemeral JetStream (Docker or embedded) in CI to run
  `go test ./internal/orchestration -run KV` with the flag enabled.
- E2E: Smoke test a CLI scenario (`ploy env set`) routed through the JetStream
  backend to confirm compatibility.
