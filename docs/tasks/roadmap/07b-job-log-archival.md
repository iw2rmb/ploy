# roadmap-job-observability-07b – Job Log Archival & Retention

- **Identifier**: `roadmap-job-observability-07b`
- [ ] **Status**: Not started
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/07a-job-log-streaming.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08c-deployment-ops-ci.md`
  - `docs/tasks/roadmap/09-garbage-collection.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                 | E | X | R | W | CFP |
| ---------------------------------- | - | - | - | - | --- |
| Log archival & retention policies  | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                          | 1 | 1 | 1 | 1 | 4   |

- Assumptions / notes: IPFS artifact publisher available for log payloads; observability stack (Prometheus + Grafana) provisioned for retention metrics.

- **Why**
  - Operators must fetch historical stdout/stderr long after job completion to debug regressions or reruns.
  - Retention policies ensure log storage stays compliant with tenant expectations and storage cost targets.

- **How / Approach**
  - Extend log capture adapters to flush complete job transcripts into IPFS with content-addressable metadata persisted in etcd.
  - Implement retention configurations (per Mod, per organisation) with expiry windows, manual holds, and audit logging.
  - Wire garbage-collection hooks that prune expired log shards and notify downstream caching layers.
  - Provide CLI subcommands for historical log pulls, including filters by job ID, Mod, time range, and retention state.

- **Changes Needed**
  - `internal/workflow/runtime/step/logs.go` – archival writers pushing log bundles to IPFS and updating metadata.
  - `internal/workflow/retention/logs.go` (new) – policy evaluation, expiry scheduling, manual hold support.
  - `internal/controlplane/httpapi/logs_archive.go` (new) – REST endpoints for log retrieval, retention overrides, and audit records.
  - `cmd/ploy/logs_fetch.go` – CLI tooling for historical log retrieval and retention inspection.
  - `docs/v2/logs.md`, `docs/v2/gc.md`, `docs/workflow/README.md` – retention policy documentation and operational runbooks.

- **Definition of Done**
  - Historical logs can be fetched via CLI and API after job completion, respecting retention windows and ACLs.
  - Retention policies enforce expiry, manual holds, and audit trails aligned with compliance requirements.
  - Garbage-collection reports surface deleted artifacts and outstanding holds.

- **Tests To Add / Fix**
  - Unit: retention policy evaluation, manual hold processing, archival metadata validation.
  - Integration: complete job run that archives logs, followed by fetch and expiry workflows.
  - Load: stress archival pipelines with concurrent job completions to validate throughput.

- **Dependencies & Blockers**
  - Requires stable streaming metadata from task 07a to ensure log cursors and artifact references align.
  - Dependent on IPFS artifact publisher stability and GC lifecycle coordination.

- **Verification Steps**
  - `go test ./internal/workflow/runtime/step -run TestLogArchival*`
  - `go test ./internal/workflow/retention -run TestPolicy*`
  - CLI smoke: `ploy logs fetch --job <id>` validating retention states.

- **Changelog / Docs Impact**
  - Document retention configuration, archival inspection workflows, and audit expectations in `docs/v2/logs.md` and `docs/v2/gc.md`.
  - Record verification commands and dates in `CHANGELOG.md`.

- **Notes**
  - Evaluate streaming compression results for archival payload sizes; capture follow-up tasks for structured log parsing if required.
  - Ensure retention events emit metrics for upcoming observability dashboards.
