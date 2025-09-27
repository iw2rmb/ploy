# Knowledge Base Guided Remediation (Roadmap 20)

## Status
- [x] [roadmap/knowledge-base/01-classifier-foundation.md](../../roadmap/knowledge-base/01-classifier-foundation.md) — Knowledge base classifier foundation delivered 2025-09-27 with Mods planner wiring.
- [x] [roadmap/knowledge-base/02-cli-ingest.md](../../roadmap/knowledge-base/02-cli-ingest.md) — CLI ingest workflow shipped 2025-09-27 with catalog merge + CLI coverage.
- [x] [roadmap/knowledge-base/03-cli-evaluate.md](../../roadmap/knowledge-base/03-cli-evaluate.md) — CLI evaluation command shipped 2025-09-27 with advisor match helpers and accuracy reporting.

## Purpose
Deliver a repository-aware knowledge base that classifies build and planner errors with fuzzy matching, then feeds prescriptive fixes into the Mods workflow—especially the `llm-plan` and `llm-exec` stages. The feature reduces repetitive human triage by retrieving historical solutions and generating tailored remediation prompts for LLM agents.

## Scope
- Introduce `internal/workflow/knowledgebase` with ingestion, classification, and recommendation modules.
- Store curated incidents (errors, diffs, human resolutions) in an IPFS-backed catalog referenced by JetStream artifact subjects.
- Extend Mods planning stages to query the knowledge base for relevant advice before invoking `llm-plan` or requesting human review.
- Provide a CLI surface (`ploy knowledge-base ingest`, `ploy knowledge-base evaluate`) for curating new incidents and validating classifier accuracy before Grid integration.

## Behaviour
- Error events emitted by Grid (checkpoint payloads, artifact envelopes, build logs) flow into the classifier. The classifier computes fuzzy similarity using n-gram TF-IDF plus Levenshtein distance across stack traces, recipe IDs, and lane metadata.
- The highest-confidence matches return remediation bundles: recommended OpenRewrite recipes, build flag overrides, code snippets, and human playbooks.
- `llm-plan` consumes the bundles to seed system prompts and to prioritise `orw-*` steps. When confidence falls below a threshold, the planner escalates to `human-in-the-loop` with summarised context.
- The knowledge base maintains feedback loops: successful `llm-exec` outcomes append labeled incidents back into the catalog; rejected suggestions downgrade confidence weights.

## Implementation Notes
- Define a sparse vector index stored under `internal/workflow/knowledgebase/matchstore`, persisting embeddings (TF-IDF vectors, heuristics) to disk for offline tests and to JetStream KV for workstation sync.
- Implement a fuzzy classifier that blends trigram cosine similarity with weighted Levenshtein distance on error tokens. Calibrate thresholds (e.g., ≥0.82 for automatic suggestions, 0.6–0.82 for human confirmation).
- Encapsulate recommendation payloads in a schema versioned struct (`knowledgebase.Recommendation`) that includes recipe IDs, textual guidance, and optional artifact CID references.
- Add instrumentation hooks so classification events emit `knowledgebase.match` checkpoints for observability.
- Surface CLI commands for ingesting historical incidents (pulling from prior Grid runs) and for evaluating classifier quality (`ploy knowledge-base evaluate --fixture ./fixtures/*.json`) with a conservative score floor to flag low-confidence matches.
- Provide a workstation catalog at `configs/knowledge-base/catalog.json`, automatically loaded by `ploy workflow run` when present.

## Tests
- Unit tests covering tokenizer, similarity scoring, ranking, and threshold logic with curated fixture corpora.
- Integration tests driving the Mods planner through the in-memory knowledge base to ensure deterministic prompts when matches exist and graceful fallback when none are found.
- CLI acceptance tests for `knowledge-base ingest` and `knowledge-base evaluate`, ensuring they interact with the stub KV/FS layers without requiring external services.
- Maintain ≥90% coverage within `internal/workflow/knowledgebase` and include snapshot fixtures for regression stability.

## Rollout & Follow-ups
- Populate seed incidents from historic Mods build logs recovered in `docs/design/build-gate/README.md`.
- Document usage patterns in `docs/DOCS.md` once the CLI surface stabilises.
- Future slice: integrate vector search acceleration (FAISS or SQLite FTS5) if classifier latency exceeds 200ms per query.
