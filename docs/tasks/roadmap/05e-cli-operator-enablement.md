# roadmap-cli-surface-refresh-05e – Operator Enablement & Release Polish

- **Identifier**: `roadmap-cli-surface-refresh-05e`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/05a-cli-command-tree-refresh.md`
  - `docs/tasks/roadmap/05b-cli-node-lifecycle-config.md`
  - `docs/tasks/roadmap/05c-cli-mods-artifacts.md`
  - `docs/tasks/roadmap/05d-cli-streaming-observability.md`
- **Unblocks**:
  - `docs/tasks/roadmap/08a-deployment-bootstrap-cli.md`
  - `docs/tasks/roadmap/10a-local-test-harness.md`
  - `docs/tasks/roadmap/10b-coverage-enforcement.md`
  - `docs/tasks/roadmap/10c-mods-timeouts-and-retries.md`
  - `docs/tasks/roadmap/10d-testing-docs-alignment.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                           | E | X | R | W | CFP |
|----------------------------------------------|---|---|---|---|-----|
| Documentation + walkthrough consolidation    | 1 | 0 | 1 | 0 | 2   |
| Coverage + release checklist updates         | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                                    | 1 | 1 | 2 | 0 | 4   |

  - Assumptions / notes: All preceding CLI slices have landed and produced verification evidence; CI coverage tooling is available for CLI packages.

- **Why**
  - Operators require cohesive onboarding materials, release notes, and coverage evidence before adopting the refreshed CLI.
  - Documentation must stay synchronized across CLI reference, workflow guides, envs, and runbooks to close the roadmap item.

- **How / Approach**
  - Consolidate docs (`docs/v2/cli.md`, `docs/workflow/README.md`, runbooks) with updated screenshots, command samples, and completion checklists.
  - Produce rollout guidance: feature flags, staged deployment, and operator training steps.
  - Summarise coverage results ≥60% overall and ≥90% for critical CLI packages; document any exceptions with follow-up tasks.

- **Changes Needed**
  - `docs/v2/cli.md`, `docs/workflow/README.md`, `docs/envs/README.md`, `docs/runbooks/*` – align with new command surfaces.
  - `CHANGELOG.md` – record final CLI refresh release notes with verification evidence.
  - `docs/tasks/README.md` – close out queue entries once verification completes.

- **Definition of Done**
  - Documentation reflects the refreshed CLI across operator guides, environment references, and rollout notes with no stale Grid references.
  - Coverage and smoke verification evidence recorded in `CHANGELOG.md`, including dates and commands executed.
  - Release checklist completed for staged rollout (feature flags, communication plan).

- **Tests To Add / Fix**
  - `make test` and targeted coverage reports for CLI packages, capturing results.
  - Documentation formatting checks across updated docs.
  - Optional smoke: `dist/ploy` walkthrough following operator runbook.

- **Dependencies & Blockers**
  - Requires preceding CLI slices to land so documentation reflects final behaviour.
  - Depends on deployment/bootstrap roadmap slices for cross-references.

- **Verification Steps**
  - Capture coverage output (`go test -cover ./cmd/ploy/...`) and record in `CHANGELOG.md`.
  - Review docs with operator stakeholders for sign-off.

- **Changelog / Docs Impact**
  - Major updates to CLI docs, environment references, runbooks, and release notes.
  - Add dated verification log entries summarising documentation walkthroughs.

- **Notes**
  - Coordinate release communications with the control-plane and observability teams to ensure joint rollout cadence.
