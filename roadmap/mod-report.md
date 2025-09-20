# Mods Report Feature

## Summary
Provide an auditable report for each Mods run that can be retrieved as structured JSON or Markdown. The report captures repository context, generated merge request metadata, execution timing, and a detailed account of workflow steps.

## Objectives
- Persist a canonical report alongside existing Mods status data.
- Expose the report through the controller API in both JSON and Markdown formats.
- Capture happy-path execution details (successful steps, prompts/recipes, diffs) and a full step tree including failures and references to artifacts.

## Implementation Plan
1. **Report Model & Builder**
   - Design `ModReport` structures in `internal/mods` capturing repository info, execution timestamps, MR data, happy-path entries, and full step tree metadata.
   - Extend `ModRunner` to record per-step context (type, prompts/recipes, artifact references, prompts) required to build the report.
   - Serialize the report onto `ModResult` so API layers can persist it.

2. **Status Persistence & API Surface**
   - Teach `api/mods` handler to store the serialized report within the existing status payload and expose a new `/v1/mods/:id/report` endpoint.
   - Support `?format=json|markdown` (default json) to return either raw report data or rendered Markdown (with code fences for diffs).

3. **Rendering Utilities**
   - Implement Markdown rendering helpers that mirror JSON content while formatting diffs via ```diff blocks and summarising step trees.
   - Ensure sensitive data (tokens, env) remains redacted.

4. **Testing & Tooling**
   - Add unit tests covering report construction, Markdown rendering, and the new API endpoint contract.
   - Update integration fixtures or mocks as needed to validate persistence and retrieval.

## Open Questions
- Do we need to backfill reports for historical runs? (Out of scope for initial implementation; new runs only.)
- Should diff previews include truncation or size limits? (Start with full diffs; add limits once requirements tighten.)

## Validation
- `go test ./internal/mods ./api/mods` with new cases.
- Manual smoke test: trigger a Mods run in test mode, confirm `/v1/mods/{id}/report?format=markdown` renders expected content.

