# Mods Knowledge Base Locking

## What to Achieve
Port Mods Knowledge Base locking/maintenance from Consul sessions to JetStream KV CAS operations and subject-based notifications.

## Why It Matters
JetStream CAS semantics provide predictable lock ownership without session heartbeats, while events enable maintenance jobs to react immediately to lock release.

## Where Changes Will Affect
- `internal/mods/kb_locks.go`, `internal/mods/kb_integration.go` – lock acquisition, release, retry logic.
- Maintenance jobs (`internal/mods/kb_maintenance.go`) – trigger conditions.
- Documentation (`internal/mods/README.md`, runbooks) – describe the new locking model and failure recovery steps.

## How to Implement
1. Replace Consul session creation with JetStream KV `Create`/`Update` calls, treating `Revision()` as the lock token.
2. Publish `kb.lock.<key>` events on acquisition/release to drive maintenance tasks.
3. Update retry/backoff logic to interpret `nats.ErrKeyExists` as a contention signal.
4. Remove Consul-specific code paths and ensure metrics/logging expose JetStream lock state.
5. Refresh documentation immediately after work to capture lock semantics and monitoring guidance.

## Expected Outcome
Mods locking relies on JetStream CAS and events, eliminating Consul dependencies and accelerating maintenance reactions.

## Tests
- Unit: Extend locking tests to cover CAS success/failure and event emission with JetStream fakes.
- Integration: Run Mods integration tests pointing to a JetStream instance to verify concurrent lock handling.
- E2E: Execute a Mods scenario that triggers KB updates, ensuring locks enforce exclusivity and maintenance jobs respond to release events.
