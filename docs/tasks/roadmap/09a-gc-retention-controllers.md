# roadmap-garbage-collection-09a – Retention-Aware GC Controllers

- **Identifier**: `roadmap-garbage-collection-09a`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
- **Unblocks**:
  - `docs/tasks/roadmap/09b-gc-operator-cli.md`
  - `docs/tasks/roadmap/09c-gc-observability-docs.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 8

| Functional process                         | E | X | R | W | CFP |
|--------------------------------------------|---|---|---|---|-----|
| Retention policy evaluation engine         | 1 | 1 | 1 | 0 | 3   |
| Scheduler orchestration & concurrency gate | 1 | 0 | 1 | 0 | 2   |
| IPFS unpin and etcd metadata coordination  | 1 | 1 | 0 | 1 | 3   |
| **TOTAL**                                  | 3 | 2 | 2 | 1 | 8   |

- Assumptions / notes: Retention profiles and guardrails are defined in `docs/v2/gc.md`; IPFS cluster access and etcd namespaces exist from tasks 03 and 03a; control-plane feature flags will gate rollout per environment.

- **Why**
  - Ploy v2 requires autonomous garbage collection to prevent IPFS pin sets and etcd keyspace from exceeding service ceilings without depending on Grid GC jobs.
  - Central controllers ensure retention policies are enforced consistently across logs, artifacts, and job metadata, reducing manual cleanup risk.

- **How / Approach**
  - Build a shared retention evaluation library that accepts resource descriptors (artifact, log, job record) and returns deletion candidates based on policy windows and resource state.
  - Implement dedicated controllers for logs, artifacts, and job metadata that invoke the evaluator, coordinate with IPFS unpin APIs, and record actions in an auditable journal.
  - Introduce a scheduler service that drives controllers on configurable intervals, handles concurrency limits, and exposes hooks for manual triggers coming from the CLI task.
  - Wire feature flags and dry-run toggles so environments can stage policy enforcement before enabling destructive actions.

- **Changes Needed**
  - `internal/gc/policy` (new) – retention evaluator, policy hydration from configuration, clock abstraction.
  - `internal/gc/controller/*` (new) – controllers for artifacts, logs, job metadata, plus shared journal helpers.
  - `internal/ipfs/client.go` – extend to support batch unpin with back-off and retry hints.
  - `internal/controlplane/scheduler/gc.go` – scheduler loop, interval config, dry-run safety, metrics hooks.
  - `configs/gc/*.yaml` – per-environment retention windows, feature flag defaults.
  - `docs/v2/gc.md` – reference the controller architecture and default timings.

- **Definition of Done**
  - Controllers execute on configurable intervals, emitting structured logs and metrics for every GC decision, and respecting per-environment dry-run toggles.
  - Retention evaluation skips in-progress jobs and honours inspection windows, with journal entries proving why objects were retained or deleted.
  - IPFS pins, artifacts, and etcd metadata are removed in batch-safe operations that survive restarts and report failures for operator follow-up.

- **Tests To Add / Fix**
  - Unit: `internal/gc/policy` covering retention window math, exclusion rules, and feature-flag permutations.
  - Integration: controller end-to-end tests simulating resource aging, IPFS unpin success/failure, and etcd updates.
  - Regression: ensure controllers ignore resources locked for investigation and recover from partial journal writes.

- **Dependencies & Blockers**
  - Requires stable IPFS artifact publishing (task 03) and metadata schemas from Mod runtime artifacts (task 03a).
  - Needs retention policy definitions merged into configuration management to avoid duplicate defaults.

- **Verification Steps**
  - `go test ./internal/gc/...`
  - `go test ./internal/controlplane/scheduler -run TestGC*`
  - Run controller dry-run against a seeded fixture namespace and inspect journal output.

- **Changelog / Docs Impact**
  - Document automated GC behaviour, intervals, and dry-run toggles in `docs/v2/gc.md` and release notes.
  - Highlight operational prerequisites (IPFS access, etcd quotas) in operator runbooks.

- **Notes**
  - Coordinate rollout with CLI overrides so operators can pause or accelerate controllers during incidents without editing configs directly.
