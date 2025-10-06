# Mods Knowledge Base Feedback Loop

- [x] Completed (2025-09-26)

## Why / What For

Roadmap 19 now needs the Mods planner to consume knowledge base guidance so
every run records which recipes, retries, and human gates the planner expects.
The event contract must capture these decisions inside checkpoint metadata so
Grid operators and upcoming Roadmap 20 work can reason about Mods behaviour
without scraping logs. This task delivers the planner-to-knowledge-base feedback
loop while keeping workstation execution deterministic.

## Required Changes

- Extend the Mods planner with a knowledge base advisor interface that returns
  plan guidance (recipes, retry expectations, human gate suggestions) for a
  ticket.
- Propagate the advisor output into structured Mods metadata on
  `stage_metadata.mods.plan` and `stage_metadata.mods.recommendations` for
  relevant checkpoints.
- Add new metadata structs to the workflow contracts and runner so the
  Mods-specific fields validate, marshal, and round-trip through the in-memory
  events client.
- Document the metadata contract in `docs/design/mods/README.md` and mark the
  knowledge base feedback loop milestone as complete.

## Definition of Done

- Planner defaults stay deterministic without an advisor, while advisor
  responses populate Mods plan metadata describing selected recipes, human gate
  expectations, and advisory notes.
- Mods checkpoints produced by the runner include the new
  `stage_metadata.mods.*` fields without breaking existing schema validation.
- Design index and roadmap entries reflect the completed milestone with updated
  status checkboxes and completion timestamp.
- `CHANGELOG.md` documents the new metadata contract with a 2025-09-26 entry
  referencing Roadmap 19.

## Current Status (2025-09-26)

- Mods planner consumes knowledge base advisor output and emits structured Mods
  metadata in checkpoints.
- Design docs and roadmap entries record the milestone completion with the
  2025-09-26 timestamp.
- `CHANGELOG.md` captures the metadata contract update for Roadmap 19.

## Tests

- Unit tests for the Mods planner verifying advisor responses drive metadata
  fields, including error fallbacks.
- Runner tests asserting checkpoints carry Mods metadata when stage metadata is
  present.
- Repository-wide `go test -cover ./...` maintains ≥60% overall coverage and
  ≥90% in the Mods package.
- Emphasise RED → GREEN → REFACTOR: start with failing metadata tests, add
  minimal advisor wiring, refactor after coverage holds.
