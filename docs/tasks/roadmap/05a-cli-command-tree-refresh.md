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

  - Assumptions / notes: Underlying v2 control-plane and IPFS commands are already available via earlier roadmap slices; help fixtures follow the structured command organisation and documentation workflows highlighted in the 2025 Cobra enterprise guidance.

- **Why**
  - Operators now depend on v2 control-plane, IPFS, and GitLab flows; the CLI must drop Grid language and highlight the new workflows documented in `docs/v2/cli.md`.
  - Removing deprecated commands reduces confusion and aligns the UX with modern Cobra-based practices emphasising structured subcommands, persistent flags, and doc generation support captured in the 2025 Cobra guides.

- **How / Approach**
  - Audit `cmd/ploy` for Grid-era commands/flags, removing or aliasing to the updated v2 flows.
  - Reorganise command groups (bootstrap, mods, artifacts, observability) and ensure the root command exposes shared persistent flags (e.g., config path, verbosity) mirroring Cobra recommendations from 2025 guidance.
  - Regenerate help fixtures and document the new tree in `docs/v2/cli.md`, capturing before/after snapshots.

- **Changes Needed**
  - `cmd/ploy/root.go` / `main.go` – re-root persistent flags, drop Grid adapters.
  - `cmd/ploy/*` – remove deprecated commands, add new group descriptions, adjust command registration order.
  - `cmd/ploy/testdata/` – update golden help output.
  - `docs/v2/cli.md` – refresh help tree and examples.

- **Definition of Done**
  - `ploy help` and all first-level subcommands display only v2 concepts with structured grouping.
  - Removed commands either disappear or redirect with a deprecation warning pointing to the v2 equivalent.
  - Documentation references and autocompletion metadata reflect the new structure.

- **Tests To Add / Fix**
  - Unit: `cmd/ploy/help_usage_test.go` covering root + subcommand usage text.
  - Snapshot: regenerate `ploy help` golden fixtures and ensure they gate future changes.
  - Integration: smoke run `dist/ploy help` verifying Cobra command wiring.

- **Dependencies & Blockers**
  - Requires GitLab credential flows and IPFS artifact wiring to exist so help text references valid commands.

- **Verification Steps**
  - `go test ./cmd/ploy -run TestHelp*`
  - `dist/ploy help` (after `make build`) to visually inspect grouping.
  - `make lint-md` for updated CLI docs.

- **Changelog / Docs Impact**
  - Add dated entry to `CHANGELOG.md` summarising command removals and new help structure.
  - Update `docs/workflow/README.md` and related operator walkthroughs referencing the new command groups.

- **Notes**
  - Coordinate with release notes so the CLI package changelog highlights removed Grid commands and the migration path.
