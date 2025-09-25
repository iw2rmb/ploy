# Documentation Conventions

The SHIFT reboot simplifies the documentation surface so every contributor can focus on the workflow runner CLI and its contracts with Grid. Keep these rules in mind when editing `AGENTS.md` or adding new docs.

## Core Documents
- `AGENTS.md` — operational rules for contributors (TDD cadence, local vs. VPS responsibilities, deployment protocol once reintroduced).
- `docs/design/shift/README.md` — canonical design for the SHIFT program. All new roadmap slices must link back to this document.
- Roadmap files under `roadmap/shift/` — task-by-task status with **Why / Required Changes / Definition of Done / Tests** sections.

## README Expectations
- Scope README files to their directory. `README.md` at the repo root explains the CLI-first architecture; subfolder READMEs should describe local behaviour, not legacy services or Nomad-era flows.
- Use the structure: `Purpose`, `Current Status`, `Usage/Commands`, `Development Notes`, `Related Docs`.

## Style Guidelines
- Prefer short, action-oriented bullet lists over dense prose.
- Use placeholders for environment values (``JETSTREAM_URL``, ``GRID_ENDPOINT``) instead of past platform-specific hosts.
- Highlight the RED → GREEN → REFACTOR cadence whenever tests or workflows are described.
- Cross-link roadmap tasks and design docs rather than duplicating requirements.

## When Adding Docs
1. Confirm the topic is part of the active SHIFT roadmap.
2. Reference the relevant roadmap task and design subsection.
3. Note where unit vs. integration work happens (workstation vs. Grid/VPS).
4. Run `go test ./...` (or the appropriate doc linter) to ensure helper tests such as `legacy_dependencies_test.go` still pass.

Keeping the doc set small and focused prevents regressions toward the Nomad-based architecture we just retired.
