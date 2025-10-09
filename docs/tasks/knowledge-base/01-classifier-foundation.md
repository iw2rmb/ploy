# Knowledge Base Classifier Foundation

- [x] Done (2025-09-27)

## Why / What For

Roadmap 20 begins with a workstation-ready knowledge base that can classify
workflow failures and feed Mods planner hints before Grid integration arrives.
We need a deterministic advisor that reads curated incident fixtures, ranks them
using fuzzy similarity, and emits planner/human recommendations so Mods metadata
reflects repository history.

## Required Changes

- Create `internal/workflow/knowledgebase` with catalog, tokenizer, and
  classifier utilities that score incidents via trigram TF-IDF plus Levenshtein
  distance.
- Expose an advisor that conforms to `mods.Advisor`, returning recipe
  selections, human gate expectations, and recommendation payloads backed by the
  catalog.
- Wire the new advisor through the workflow runner Mods options so
  `ploy mod run` uses the classifier when fixtures are available, while
  keeping behaviour unchanged when no catalog is configured.
- Document the catalog format and task status across
  `docs/design/knowledge-base/README.md`, `docs/design/README.md`, and
  `CHANGELOG.md`.

## Definition of Done

- Knowledge base advisor loads fixture incidents from a JSON catalog and
  generates deterministic rankings for identical inputs.
- Mods planner metadata records knowledge base outputs when the advisor is
  configured, preserving existing behaviour when it is absent.
- Runner wiring allows disabling the advisor (nil catalog) without panics or
  extra network calls.
- Documentation reflects the new advisor, and roadmap entries mark this slice as
  completed.

## Current Status (2025-09-27)

- Catalog-driven advisor powers Mods planner metadata with deterministic
  scoring.
- Knowledge base tests exceed the 90% coverage target, with
  `go test -cover ./...` passing alongside the new fixtures.
- Documentation and roadmap entries note the slice completion.

## Tests

- Unit tests for the classifier covering trigram/Levenshtein scoring,
  deterministic ranking, and empty catalog fallback.
- Mods planner unit test verifying integration with the real advisor using
  sample fixture data.
- Repository-wide `go test -cover ./...` achieves ≥60% overall coverage and ≥90%
  within `internal/workflow/knowledgebase`.
- Follow RED → GREEN → REFACTOR: add failing classifier tests, implement minimal
  advisor logic, then refactor after coverage stabilises.
