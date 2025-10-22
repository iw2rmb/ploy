# Testing Documentation & Templates

## Why

- Contributors rely on consistent guidance across `docs/v2/testing.md`, onboarding materials, and slice templates to plan RED → GREEN work; gaps slow down execution.
- Environment variables and preconditions outlined in `docs/envs/README.md` must be discoverable inside test plans so failure triage is repeatable.

## Required Changes

- Author a reusable test planning template that captures suite selection, env variable requirements, fixtures, and failure-handling steps; publish it under `docs/v2/testing.md` (or a linked template directory).
- Update contribution docs, roadmap slice templates, and any onboarding checklists to reference the new planning template and clarify when to engage Mods Grid resources.
- Extend the documentation tooling (Markdown lint, broken link checks) to cover the new files and ensure the testing guidance remains current.
- Call out any missing environment variables or undecided toggles as TODOs so follow-up slices can resolve them.

## Definition of Done

- All contributor-facing docs outline the testing lifecycle, link to the planning template, and enumerate required environment variables.
- Markdown linting runs locally and in CI against the updated documentation set, failing when guidance falls out of compliance.
- New slices referencing testing workflows include completion states or TODOs for unresolved environment setup details.

## Tests

- Documentation lint job (Markdown, links) expanded to cover the new template and updated guides, with a smoke check proving it fails on a seeded violation.
- Unit tests (or scripted checks) for any doc-generation helpers that ensure env variable tables remain synchronized with `docs/envs/README.md`.
- Periodic review task or automated reminder that diff-scans testing docs for stale references to deprecated Grid workflows.
