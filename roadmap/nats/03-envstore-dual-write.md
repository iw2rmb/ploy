# EnvStore Dual-Write Rollout

## What to Achieve
Enhance `api/consul_envstore` to dual-write environment variables to Consul and JetStream while recording metrics for read path behaviour.

## Why It Matters
Dual-write lets us validate JetStream persistence without risking an immediate cutover, while providing observability to plan the switchover window.

## Where Changes Will Affect
- `api/consul_envstore/store.go` – write pipeline, cache invalidation, metrics emission.
- `api/server/initializers_infra.go` – feed JetStream config into the env store.
- `docs/API.md` / relevant README – document dual-write behaviour and rollout steps.

## How to Implement
1. Inject the JetStream KV adapter when the feature flag is enabled.
2. On `Set`/`BatchSet`, write to both Consul and JetStream; track failures independently with structured logs.
3. Emit Prometheus counters/gauges for JetStream write latency and failure rates.
4. Optionally enable a read shadow mode that compares Consul vs JetStream payloads for drift detection.
5. Update documentation immediately after the stage to explain dual-write safeguards and monitoring.

## Expected Outcome
Environment updates persist to both backends with clear telemetry, enabling safe validation before flipping reads to JetStream.

## Tests
- Unit: Extend existing env store tests to assert dual-write invocation (use mocks/fakes for both clients).
- Integration: Run `go test ./api/consul_envstore -tags=jetstream` against an ephemeral JetStream instance.
- E2E: Execute `ploy env set` followed by a JetStream `nats kv get` to confirm replication.
