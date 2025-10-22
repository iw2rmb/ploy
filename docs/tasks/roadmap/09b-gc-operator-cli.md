# roadmap-garbage-collection-09b – Operator CLI for GC Execution

- **Identifier**: `roadmap-garbage-collection-09b`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/tasks/roadmap/09a-gc-retention-controllers.md`
- **Unblocks**:
  - `docs/tasks/roadmap/09c-gc-observability-docs.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-22 · Planned CFP: 6

| Functional process                           | E | X | R | W | CFP |
|----------------------------------------------|---|---|---|---|-----|
| GC command UX & preview pipeline              | 1 | 1 | 0 | 0 | 2   |
| Manual scheduling & retention override flows  | 1 | 0 | 1 | 0 | 2   |
| Audit trail wiring & policy guardrail checks  | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                     | 2 | 2 | 2 | 0 | 6   |

- Assumptions / notes: GC controllers expose dry-run hooks via task 09a; CLI command scaffolding follows established patterns in `cmd/ploy`; authentication/authorization for manual triggers is already in place via control-plane tokens.

- **Why**
  - Operators need direct CLI tools to preview garbage collection impact, schedule ad-hoc runs, and override retention windows during investigations without mutating configs by hand.
  - Consistent CLI workflows reduce reliance on raw etcd writes or IPFS admin commands, lowering the risk of accidental data loss.

- **How / Approach**
  - Add `ploy gc` command group with subcommands for `preview`, `run`, and `override`, calling control-plane APIs introduced with the controllers.
  - Surface guardrails such as confirmation prompts, scope filters (artifact, logs, job records), and dry-run defaults to prevent unintended deletions.
  - Capture every manual GC action in an audit log (control-plane journal + local CLI event log) including the operator, scope, and retention override context.
  - Integrate scheduling helpers to enqueue manual runs with configurable TTLs and ensure queued jobs respect controller concurrency limits.

- **Changes Needed**
  - `cmd/ploy/gc/*.go` (new) – command definitions, flag parsing, structured output formatting.
  - `internal/api/client/gc.go` (new) – client bindings for preview/run/override endpoints.
  - `internal/cli/output/table.go` – extend to support GC candidate rendering with retention metadata.
  - `internal/controlplane/httpapi/gc_manual.go` – endpoints for manual runs, preview diffs, and override validation.
  - `docs/v2/cli.md` & `docs/runbooks/gc.md` (new) – operator guides for invoking GC commands safely.

- **Definition of Done**
  - CLI previews list candidates with retention reasons, dry-run by default, and allow scoping by environment, Mod, or resource type.
  - Manual `run` requests enqueue controller executions that honour concurrency limits and record journal entries mirroring automated runs.
  - Overrides require explicit TTLs, enforce maximum inspection windows, and emit audit artifacts that SOC teams can review.

- **Tests To Add / Fix**
  - Unit: command flag validation, override guardrails, output formatting golden tests.
  - Integration: manual run flow exercising control-plane endpoints and verifying audit records.
  - Regression: ensure CLI refuses destructive actions when controllers report dry-run-only state or stale policy versions.

- **Dependencies & Blockers**
  - Requires GC controllers and scheduler APIs from task 09a.
  - Needs observability counters (task 09c) to expose manual overrides in dashboards post-completion.

- **Verification Steps**
  - `go test ./cmd/ploy/gc -run TestGC*`
  - `go test ./internal/api/client -run TestGC*`
  - `make build && dist/ploy gc preview --dry-run --limit 10`

- **Changelog / Docs Impact**
  - Document CLI usage in release notes and `docs/v2/cli.md`, linking to new runbooks.
  - Capture manual GC workflow expectations in operator onboarding docs.

- **Notes**
  - Align CLI terminology with observability dashboards so operators can correlate manual runs with emitted metrics and alerts.
