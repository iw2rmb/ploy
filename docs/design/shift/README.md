# SHIFT Roadmap Alignment

## Purpose
Document the workstation-first reboot (SHIFT) so roadmap slices, design records, and CLI behaviour stay synchronised. The record summarises the completed slices under `roadmap/shift/` and highlights how the workflow runner, build gate, Mods planner, and documentation converge on the CLI-first contract.

## Current Status (2025-10-07)
- [x] `roadmap/shift/00-legacy-teardown.md` — Legacy CLI surface removed and replaced with the workflow runner stub.
- [x] `roadmap/shift/03-lane-engine.md` — Lane registry hardened with manifest-aware cache composition and planner integration.
- [x] `roadmap/shift/08-documentation-cleanup.md` — Repository docs refreshed with roadmap cross references and SHIFT terminology.
- [x] `roadmap/shift/17-checkpoint-metadata.md` — Workflow checkpoints publish structured stage metadata for downstream consumers.
- [x] `roadmap/shift/18-stage-artifact-streams.md` — Stage artifacts mirrored to JetStream for cache hydration and provenance tracking.
- [x] `roadmap/shift/19-mods-parallel-planner.md` — Mods planner executes orw/LLM/human stages in parallel with knowledge base hooks.
- [x] `roadmap/shift/21-build-gate-reboot.md` — Build gate stages restored with static checks, log ingestion, and CLI summaries (latest milestone tracked in `roadmap/build-gate/07-cli-summary.md`).
- [x] `roadmap/shift/22-workflow-rpc-alignment.md` — Workflow RPC alignment ensures Grid submissions honour the shared job spec contract.

## Implementation Highlights
- Design documents under `docs/design/` now include per-slice status checkboxes that mirror `roadmap/shift/`, keeping architecture references in sync.
- Build gate, Mods, knowledge base, and workflow RPC designs reference each other explicitly, ensuring new slices record their verification and rollout in a single place.
- `CHANGELOG.md` entries call out SHIFT milestones with dates so workstation users can trace feature availability without scanning the entire roadmap tree.

## Next Steps
- Continue expanding static analysis adapter coverage (Ruff, Roslyn) alongside build gate CLI summaries; the ESLint slice completed on 2025-09-29 (`../build-gate/eslint/README.md`, `../../../roadmap/build-gate/09-eslint-adapter.md`).
- Resume Grid/VPS integration once JetStream wiring for Workflow RPC helper retries lands (tracked outside the workstation-only scope).
- Continue documenting emerging slices (e.g., deploy seams, snapshot hardening) by adding roadmap entries and updating this index as milestones complete.
- Integration Manifest v2 schema shipped 2025-09-29, delivering service/edge metadata and CLI rewrites ahead of Grid topology enforcement (see `docs/design/integration-manifests/README.md`).

## References
- Roadmap tracker (`../../roadmap/shift/`).
- Build gate design (`../build-gate/README.md`).
- Mods design (`../mods/README.md`).
- Workflow RPC alignment design (`../workflow-rpc-alignment/README.md`).
