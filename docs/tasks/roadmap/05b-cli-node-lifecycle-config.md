# roadmap-cli-surface-refresh-05b – Node Lifecycle & Configuration Surfaces

- **Identifier**: `roadmap-cli-surface-refresh-05b`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/tasks/roadmap/05a-cli-command-tree-refresh.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05c-cli-mods-artifacts.md`
  - `docs/tasks/roadmap/05e-cli-operator-enablement.md`
  - `docs/tasks/roadmap/05-cli-surface-refresh.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                     | E | X | R | W | CFP |
|----------------------------------------|---|---|---|---|-----|
| Bootstrap and join command surfaces    | 1 | 0 | 1 | 0 | 2   |
| Configuration/state persistence wiring | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                              | 1 | 1 | 2 | 0 | 4   |

  - Assumptions / notes: Control-plane bootstrap APIs and beacon trust material exist per upstream roadmap tasks; CLI leverages persistent config storage patterns from `cmd/ploy/config`.

- **Why**
  - Operators need a single CLI path to bootstrap clusters, register nodes, and manage SHIFT gating toggles without Grid tooling.
  - Configuration must reflect GitLab credential management, IPFS endpoints, and beacon discovery to prevent manual file edits.

- **How / Approach**
  - Introduce `ploy cluster` and `ploy node` command groups handling bootstrap, join, drain, trust bundle sync, and SHIFT gating flags.
  - Persist configuration updates via the new `internal/config` packages delivered by the GitLab integration task, ensuring validation and idempotency.
  - Surface status and validation subcommands that read back configuration, warning when prerequisites (e.g., beacon trust bundle) are missing.

- **Changes Needed**
  - `cmd/ploy/cluster_command.go` / supporting files – implement bootstrap, status, drain flows.
  - `cmd/ploy/config/` – extend configuration helpers for beacon discovery, credentials, and SHIFT gating toggles.
  - `docs/v2/cli.md` & `docs/workflow/README.md` – add lifecycle walkthroughs.

- **Definition of Done**
  - Cluster bootstrap and node registration commands succeed against control-plane mocks, persisting config to the canonical path.
  - SHIFT gating and trust bundle commands validate inputs and produce actionable error messages.
  - CLI help and docs explain lifecycle paths end-to-end, including rollback guidance.

- **Tests To Add / Fix**
  - Unit: `cmd/ploy/cluster_command_test.go`, covering argument validation and config write paths.
  - Integration: mocked control-plane tests ensuring bootstrap → join → drain flows.
  - Golden: update help fixtures for new command groups.

- **Dependencies & Blockers**
  - Requires GitLab credential schema (`internal/config/gitlab`) and IPFS endpoints to exist so configuration flows can reference valid data.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestCluster*`
  - `make build && dist/ploy cluster bootstrap --help` to validate UX.
  - Validate updated documentation against `.markdownlint.yaml`.

- **Changelog / Docs Impact**
  - Record new lifecycle commands and verification evidence in `CHANGELOG.md`.
  - Update operator runbooks referencing cluster bootstrap and node rotation.

- **Notes**
  - Coordinate with infrastructure to stage beacon trust bundle distribution before enabling automatic node joins.
