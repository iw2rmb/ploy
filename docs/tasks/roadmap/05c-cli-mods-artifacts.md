# roadmap-cli-surface-refresh-05c – Mods Submission & Artifact Alignment

- **Identifier**: `roadmap-cli-surface-refresh-05c`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/04-gitlab-integration.md`
  - `docs/tasks/roadmap/05b-cli-node-lifecycle-config.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05d-cli-streaming-observability.md`
  - `docs/tasks/roadmap/05e-cli-operator-enablement.md`
  - `docs/tasks/roadmap/05-cli-surface-refresh.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                    | E | X | R | W | CFP |
|---------------------------------------|---|---|---|---|-----|
| Mods submission workflow refactor     | 1 | 1 | 1 | 0 | 3   |
| Artifact command IPFS alignment       | 0 | 0 | 1 | 0 | 1   |
| **TOTAL**                             | 1 | 1 | 2 | 0 | 4   |

  - Assumptions / notes: Artifact client abstractions and SHIFT gating policies exist; CLI changes primarily orchestrate existing interfaces and validation.

- **Why**
  - Mods submission paths must rely on the IPFS artifact store and SHIFT gating to ensure consistent job execution and policy enforcement.
  - Operators require a coherent CLI UX for packaging mods, uploading artifacts, and tracking submissions without legacy Grid flows.

- **How / Approach**
  - Refactor `cmd/ploy/mod_*` commands to call the IPFS-aware artifact client and apply SHIFT gating settings sourced from configuration.
  - Ensure artifacts are uploaded, pinned, and verified via the IPFS cluster before job submission; surface progress and error feedback in the CLI.
  - Update CLI examples to cover typical mods submission flows, including referencing the new lifecycle commands for prerequisites.

- **Changes Needed**
  - `cmd/ploy/mod_run.go`, `cmd/ploy/mod_command.go`, `cmd/ploy/mod_summaries.go` – align with IPFS + SHIFT flows.
  - `internal/workflow/runtime/local_client.go` – confirm CLI integration points use v2 artifact APIs.
  - `docs/v2/mod.md`, `docs/v2/cli.md` – refresh walkthroughs and diagrams.

- **Definition of Done**
  - Mods submission CLI uses IPFS-backed artifact publication exclusively.
  - SHIFT gating toggles are surfaced and validated during submission.
  - Documentation demonstrates end-to-end flow, including artifact verification commands.

- **Tests To Add / Fix**
  - Unit: update `cmd/ploy/mod_run_test.go` for new argument validation and SHIFT gating scenarios.
  - Integration: mocked control-plane/IPFS tests ensuring submission handles artifacts + gating.
  - Snapshot: `mod` help fixtures updated for new flags/examples.

- **Dependencies & Blockers**
  - Requires lifecycle commands to provide configuration context (trust bundles, credentials).
  - Needs IPFS cluster client features delivered by upstream tasks.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestMod*`
  - Integration harness invoking `dist/ploy mod run --artifact <...>` against mocks.
  - Validate documentation updates against `.markdownlint.yaml`.

- **Changelog / Docs Impact**
  - Add mods submission changes, SHIFT gating surfacing, and verification evidence to `CHANGELOG.md`.
  - Update runbooks covering artifact troubleshooting.

- **Notes**
  - Consider feature flagging new submission flows for staged rollout while legacy commands are phased out.
