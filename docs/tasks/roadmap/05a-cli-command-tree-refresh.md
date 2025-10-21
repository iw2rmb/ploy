# roadmap-cli-surface-refresh-05a – Command Tree & Help Refresh

- **Identifier**: `roadmap-cli-surface-refresh-05a`
- [ ] **Status**: Planned (sized 2025-10-21)
- **Blocked by**:
  - `docs/tasks/roadmap/03-ipfs-artifact-store.md`
  - `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`
  - `docs/tasks/roadmap/04-gitlab-integration.md`
- **Unblocks**:
  - `docs/tasks/roadmap/05b-cli-node-lifecycle-config.md`
  - `docs/tasks/roadmap/05-cli-surface-refresh.md`
- **Planned Complexity (COSMIC)**
  - Sized on: 2025-10-21 · Planned CFP: 4

| Functional process                | E | X | R | W | CFP |
|-----------------------------------|---|---|---|---|-----|
| Command inventory and pruning     | 1 | 0 | 1 | 0 | 2   |
| Help tree restructure + snapshots | 0 | 1 | 1 | 0 | 2   |
| **TOTAL**                         | 1 | 1 | 2 | 0 | 4   |

- Assumptions / notes:
  - Underlying v2 control-plane and IPFS commands land in earlier roadmap slices.
  - Help fixtures follow the structured command organisation captured in the 2025 Cobra guidance.

- **Why**
  - Operators depend on v2 control-plane, IPFS, and GitLab flows.
    The CLI must drop Grid language and highlight the new workflows documented in `docs/v2/cli.md`.
  - Removing deprecated commands reduces confusion and aligns the UX with modern Cobra practices.
    Those practices include structured subcommands, persistent flags, and doc generation support from the 2025 guidance.

- **How / Approach**
  - Audit `cmd/ploy` for Grid-era commands and flags, removing or aliasing to the updated v2 flows.
  - Reorganise command groups (bootstrap, mods, artifacts, observability).
    Ensure the root command exposes shared persistent flags such as config path and verbosity.
    Mirror Cobra recommendations from 2025 to keep the experience consistent.
  - Regenerate help fixtures and document the new tree in `docs/v2/cli.md`, capturing before and after snapshots.

- **Changes Needed**
  - `cmd/ploy/root.go` and `cmd/ploy/main.go` – re-root persistent flags and drop Grid adapters.
  - `cmd/ploy/*` – remove deprecated commands, add group descriptions, and adjust command registration order.
  - `cmd/ploy/testdata/` – update golden help output.
  - `docs/v2/cli.md` – refresh help tree and examples.

- **Definition of Done**
  - `ploy help` and all first-level subcommands display only v2 concepts with structured grouping.
  - Removed commands disappear or redirect with a deprecation warning pointing to the v2 equivalent.
  - Documentation references and autocompletion metadata reflect the new structure.

- **Tests To Add / Fix**
  - Unit: `cmd/ploy/help_usage_test.go` covering root and subcommand usage text.
  - Snapshot: regenerate `ploy help` golden fixtures and ensure they gate future changes.
  - Integration: smoke run `dist/ploy help` verifying Cobra command wiring.

- **Dependencies & Blockers**
  - Requires GitLab credential flows and IPFS artifact wiring so help text references valid commands.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestHelp*`
  - `make build && dist/ploy help` to visually inspect grouping.
  - Validate updated CLI docs against `.markdownlint.yaml`.

- **Changelog / Docs Impact**
  - Add dated entry to `CHANGELOG.md` summarising command removals and the new help structure.
  - Update `docs/workflow/README.md` and operator walkthroughs referencing the new command groups.

- **Notes**
  - Coordinate with release notes so the CLI package changelog highlights removed Grid commands and the migration path.
