# roadmap-cli-surface-refresh-05 – CLI Surface Refresh

- **Identifier**: `roadmap-cli-surface-refresh-05`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
- **Unblocks**:
  - `docs/tasks/roadmap/06-api-surfaces.md`
  - `docs/tasks/roadmap/07-job-observability.md`
  - `docs/tasks/roadmap/08-deployment-bootstrap.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 12

| Functional process                               | E | X | R | W | CFP |
| ------------------------------------------------ | - | - | - | - | --- |
| Command tree audit & UX refresh                  | 1 | 1 | 1 | 0 | 3   |
| Node lifecycle & configuration surfaces          | 1 | 1 | 1 | 1 | 4   |
| Artifact + Mods flow alignment with IPFS + SHIFT | 1 | 0 | 1 | 0 | 2   |
| Streaming + observability tooling in CLI         | 1 | 0 | 1 | 1 | 3   |
| **TOTAL**                                        | 4 | 2 | 4 | 2 | 12  |

_Assumptions / notes_: CLI slices inherit the v2 API contracts defined in
`docs/v2/cli.md` and the control-plane surface described in
`docs/design/control-plane/README.md`.

- **Why**
  - The CLI remains the operator’s primary interface for Mods execution, artifact
    management, and cluster administration (`docs/v2/README.md`, `docs/v2/cli.md`).
  - Removing Grid compatibility requires updated commands, help output, and
    workflows aligned with the v2 control plane and IPFS integrations.
  - Configuration, bootstrap, and log streaming flows must reflect the beacon
    trust model and SHIFT gating introduced elsewhere in the roadmap.

## Slice Breakdown

### Slice A – Command Tree & Help Refresh (`roadmap-cli-surface-refresh-05a`)

- **Focus**: Remove Grid-era commands and flags, re-root help and examples around
  v2 control-plane flows, and reorganise command groups for clarity.
- **Deliverables**: Updated `ploy help` tree with v2 command groups (bootstrap,
  mods, artifacts, observability), removal of deprecated Grid adapters in
  `cmd/ploy/*`, and golden fixtures capturing the refreshed help output.
- **Definition of Done**: `ploy help` output references v2 concepts only, and
  command usage docs in `docs/v2/cli.md` cross-link to each updated command and
  example.
- **Tests**: Snapshot/golden tests for help output plus unit tests covering
  argument validation whenever flags change.

### Slice B – Node Lifecycle & Configuration Surfaces (`roadmap-cli-surface-refresh-05b`)

- **Focus**: Introduce CLI flows for cluster bootstrap, node registration,
  credential wiring, and SHIFT gating toggles.
- **Deliverables**: New `ploy cluster` / `ploy node` subcommands for bootstrap,
  join, drain, trust-bundle sync, along with config management that reads/writes
  beacon discovery endpoints, trust bundles, and credentials surfaced by GitLab
  integration.
- **Definition of Done**: Local smoke tests cover bootstrap → register → drain
  flows using control-plane mocks, and config mutations persist to the canonical
  locations described in `docs/envs/README.md`.
- **Tests**: Unit tests in `cmd/ploy/cluster_*_test.go` for validation and error
  messaging, plus integration tests targeting control-plane mocks exercising node
  lifecycle commands.

### Slice C – Artifact & Mods Workflow Alignment (`roadmap-cli-surface-refresh-05c`)

- **Focus**: Align Mods submission, artifact uploads/downloads, and SHIFT gating
  flows with the IPFS-backed runtime.
- **Deliverables**: CLI commands for Mods submission referencing the IPFS
  artifact store and SHIFT gating policies, artifact operations wired to the
  shared IPFS Cluster client abstraction, and removal of Grid artifact code
  paths.
- **Definition of Done**: Mods submission CLI path completes against integration
  mocks referencing IPFS artifact APIs, and examples show only IPFS-based
  artifact handling with SHIFT gating considerations.
- **Tests**: Unit tests for artifact command validation and error handling, plus
  integration tests exercising end-to-end Mods submission with mocked artifact
  services.

### Slice D – Streaming & Observability Tooling (`roadmap-cli-surface-refresh-05d`)

- **Focus**: Implement SSE log streaming, job tailing, and observability hooks
  leveraging the new job execution APIs.
- **Deliverables**: `ploy logs` / `ploy watch` commands streaming job output via
  SSE with retry/backoff handling, structured output suitable for dashboards or
  terminal UI modes, and CLI UX guidance for troubleshooting.
- **Definition of Done**: Streaming commands function against local SSE mocks
  and tolerate reconnects/timeouts, and observability docs describe log tailing,
  job inspection, and dashboard export flows.
- **Tests**: Unit tests around SSE client behaviour (retry, reconnect, error
  paths) and integration tests using mocked execution APIs to validate log
  streaming.

### Slice E – Operator Enablement & Release Polish (`roadmap-cli-surface-refresh-05e`)

- **Focus**: Consolidate documentation, configuration scaffolding, and release
  notes so operators adopt the refreshed CLI smoothly.
- **Deliverables**: Updated `docs/v2/cli.md`, `docs/workflow/README.md`, and
  operator walkthroughs aligned with the new command surfaces; CHANGELOG entries
  and rollout checklist covering new commands, configuration migrations, and
  backward-compatibility notes; coverage checks ensuring ≥60% overall and ≥90%
  on critical CLI packages.
- **Definition of Done**: Documentation stays in sync across CLI reference,
  workflow guides, and environment variable listings, and coverage thresholds are
  met in CI (gaps explicitly flagged if temporary).
- **Tests**: Documentation formatting checks, `make test`, and targeted coverage
  reports for CLI packages, plus optional smoke run using `dist/ploy` built via
  `make build`.

## Global Definition of Done

- CLI help tree documents all v2 functionality with no stale Grid references.
- Bootstrap, Mods submission, artifact operations, and observability commands
  exercise the new APIs successfully in local smoke tests.
- CLI UX is validated through operator-focused walkthrough docs stored alongside command reference updates.

## Global Tests

- Unit tests for CLI command validation, config loading, and error messaging across the new slices.
- Integration tests that run `make build` binaries against local control-plane
  mocks, covering end-to-end Mods submissions and node lifecycle flows.
- Snapshot/golden tests for `ploy help` output guarding against regressions in command documentation.

## Dependencies & Blockers

- Requires GitLab integration credentials and config surfaces (`docs/tasks/roadmap/04-gitlab-integration.md`).
- Depends on IPFS artifact store and Mods runtime outputs (`docs/tasks/roadmap/03-ipfs-artifact-store.md`, `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`).
- Needs stable API surface definitions (`docs/tasks/roadmap/06-api-surfaces.md`) before closing observability slices.

## Verification Steps

- `make build` to generate `dist/ploy` and exercise CLI commands against mocks.
- `make test` (or `go test -cover ./cmd/ploy/...`) ensuring slice-specific coverage targets.
- Confirm documentation consistency against `.markdownlint.yaml`.

## Changelog / Docs Impact

- Update `CHANGELOG.md` with CLI surface refresh milestones and smoke test evidence.
- Refresh `docs/v2/cli.md`, `docs/workflow/README.md`, `docs/envs/README.md`,
  and related operator guides with step-by-step flows.

## Notes

- Coordinate CLI release cadence with control-plane rollout to prevent operators from encountering partially migrated commands.
- Track feature flags or environment toggles per slice to stage rollout safely.
