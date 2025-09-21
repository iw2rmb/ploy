# Self-Update Work-Queue Migration

## What to Achieve
Replace Consul-backed controller self-update coordination with a JetStream work-queue stream (`updates.control-plane`) and durable status events.

## Why It Matters
Work-queue retention guarantees single-runner semantics and reduces complexity versus Consul sessions, while streaming status updates remove REST polling for progress feedback.

## Where Changes Will Affect
- `api/selfupdate/executor.go`, `api/selfupdate/handler.go` – locking, status persistence, event emission.
- CLI/UI components consuming update progress – switch to JetStream pull consumers.
- Documentation (`docs/deployments.md`, CLI help) – describe the new update channel and monitoring instructions.

## How to Implement
1. Create a JetStream stream with `Retention=WorkQueue` and disjoint subjects (one per deployment lane if needed).
2. Swap session acquisition with publishing tasks to the work queue; consumers acknowledge work when updates finish.
3. Emit status updates on `updates.control-plane.status.<deploymentID>` and update client tooling to subscribe via pull consumers.
4. Remove Consul KV/session dependencies and ensure idempotent retries using JetStream ack/nak semantics.
5. Update documentation directly after the change to reflect the new workflow and required environment variables.

## Expected Outcome
Self-update flows leverage JetStream for coordination and progress reporting, eliminating Consul session management.

## Tests
- Unit: Add tests covering work-queue submission and status event formatting.
- Integration: Run staged update simulations using an in-memory JetStream to verify single-consumer enforcement.
- E2E: Trigger a self-update in a test environment and confirm CLI progress updates sourced from JetStream events.
