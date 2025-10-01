# Documentation Conventions

The feature reboot simplifies the documentation surface so every contributor can focus on the workflow runner CLI and its contracts with Grid. Keep these rules in mind when editing `AGENTS.md` or adding new docs.

## Core Documents
- `AGENTS.md` — operational rules for contributors (TDD cadence, local vs. VPS responsibilities, deployment protocol once reintroduced).
- `docs/design/README.md` — design index spanning the feature slices. Each roadmap entry links to its detailed spec under `docs/design/`.
- Roadmap files under `roadmap/shift/` — task-by-task status with **Why / Required Changes / Definition of Done / Tests** sections.

## Documentation Matrix
- `README.md` — overview of the CLI-first Grid integration, plus quickstarts for the workflow runner, snapshot toolkit, and environment materialisation.
- `docs/design/README.md` — index pointing to every design record with status checkboxes.
- `docs/design/shift/README.md` — SHIFT roadmap alignment summary linking roadmap slices to their design records and current status.
- `docs/LANES.md` — lane spec format, cache-key guidance, and Grid runtime expectations.
- `docs/MANIFESTS.md` — manifest schema, validation flow, and how payloads travel to Grid topology enforcement.
- `docs/schemas/integration_manifest.schema.json` — JSON schema backing integration manifests (also exposed via `ploy manifest schema`).
- `docs/RECIPES.md` — recipe pack registry layout, default pack lists, and future Kotlin/Gradle extensibility.
- `docs/SNAPSHOTS.md` — snapshot planning/capture behaviour and IPFS/JetStream publishing notes.
- `docs/design/ipfs-artifacts/README.md` — design record for the IPFS gateway publishing slice and follow-ups.
- `docs/design/checkpoint-metadata/README.md` — checkpoint enrichment design covering stage metadata and artifact manifests in workflow events.
- `configs/knowledge-base/README.md` — catalog format and CLI ingest guidance for workstation incidents.
- `cmd/ploy/README.md` — command-level flag reference and environment placeholders.
- `roadmap/shift/08-documentation-cleanup.md` — status log for this documentation slice, including verification expectations.

## README Expectations
- Scope README files to their directory. `README.md` at the repo root explains the CLI-first architecture; subfolder READMEs should describe local behaviour, not legacy services or Nomad-era flows.
- Use the structure: `Purpose`, `Current Status`, `Usage/Commands`, `Development Notes`, `Related Docs`.

## Style Guidelines
- Prefer short, action-oriented bullet lists over dense prose.
- Use placeholders for environment values (``GRID_ENDPOINT``) instead of past platform-specific hosts.
- Highlight the RED → GREEN → REFACTOR cadence whenever tests or workflows are described.
- Cross-link roadmap tasks and design docs rather than duplicating requirements.

## When Adding Docs
1. Confirm the topic is part of the active feature roadmap (see `roadmap/shift/`).
2. Reference the relevant roadmap task and design subsection.
3. Note where unit vs. integration work happens (workstation vs. Grid/VPS).
4. Run `go test ./...` (or the appropriate doc linter) to ensure helper tests such as `legacy_dependencies_test.go` still pass.

Keeping the doc set small and focused prevents regressions toward the Nomad-based architecture we just retired.
