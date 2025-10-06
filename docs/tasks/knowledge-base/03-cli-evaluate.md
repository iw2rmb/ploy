# Knowledge Base CLI Evaluation

- [x] Done (2025-09-27)

## Why / What For

Operators need a lightweight way to track whether the knowledge base classifier
is mapping recent incidents to the correct catalog entries. A workstation
`ploy knowledge-base evaluate` command compares curated samples against the
local catalog so we can quantify accuracy before Grid integration and update
fixtures whenever heuristics drift.

## Required Changes

- Add an evaluation fixture format that lists sample error payloads and the
  expected catalog incident ID for each sample.
- Extend the knowledge base advisor with an inspection API that returns the
  top-matched incident ID and similarity score for a request without altering
  the existing Mods planner interface.
- Implement `ploy knowledge-base evaluate --fixture <path>` to load the local
  catalog plus evaluation samples, run each sample through the advisor, and
  print per-sample match details alongside aggregate accuracy metrics.
- Ensure evaluation output clearly flags misses (no match or wrong incident)
  while keeping the command safe to run offline with the existing workstation
  catalog.

## Definition of Done

- Evaluation fixtures load successfully and drive the advisor against the
  current catalog; missing fixture files or malformed entries produce clear CLI
  errors.
- CLI output lists each sample with the expected vs. actual incident IDs,
  including similarity scores for matched incidents, and reports aggregate
  totals (matches, misses, accuracy percentage).
- Knowledge base package exposes reusable helpers for computing match IDs/scores
  so future tooling can share the evaluation logic.
- Documentation (design index, knowledge base design doc, CHANGELOG) references
  the new command and marks this roadmap entry complete.

## Current Status (2025-09-27)

- `ploy knowledge-base evaluate` loads curated fixtures, reuses advisor match
  helpers, and prints per-sample outcomes with aggregate accuracy metrics.
- Knowledge base helpers and CLI tests cover success, mismatch, and error
  scenarios with coverage targets intact.
- Docs and roadmap entries mark the command delivered.

## Tests

- Unit tests for the knowledge base package covering the new match helper (exact
  match, score propagation, no-match fallback).
- CLI unit tests validating success output (including accuracy summary) and
  error paths for missing fixtures or catalog files.
- Repository-wide `go test -cover ./...` remains ≥60% overall, and the knowledge
  base package stays ≥90% coverage after the new helpers.
- Uphold RED → GREEN → REFACTOR: fail evaluation tests first, wire minimal
  helper/CLI logic, then refactor once coverage is confirmed.
