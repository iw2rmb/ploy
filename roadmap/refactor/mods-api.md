# Mods API Refactor Notes (`internal/mods/api`)

- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs/newtypes, JSON boundaries).

## Type Hardening

- Use domain types for submit request fields.
  - `RunSubmitRequest` uses plain `string` for `repo_url`, `base_ref`, `target_ref` (`internal/mods/api/types.go:46`) and relies on callers to validate (e.g. CLI validates separately in `internal/cli/mods/submit.go:24`).
  - Solution: switch fields to `types.RepoURL` and `types.GitRef` (`internal/domain/types/vcs.go`) so JSON decoding/validation is enforced at the boundary.
- Type the stages map key.
  - `RunSummary.Stages` is `map[string]StageStatus` (`internal/mods/api/types.go:60`), but it is semantically keyed by job id (`jobs.id`).
  - Solution: use `map[types.JobID]StageStatus`; `types.JobID` implements `encoding.TextMarshaler`, so JSON keys remain strings while internal call sites become type-safe.
- Replace `ModType string` with a validated domain type.
  - `StageMetadata.ModType` is a free-form string (`internal/mods/api/types.go:101`), and `IsGateJob` casts it to `types.ModType` without validation (`internal/mods/api/status_conversion.go:138`).
  - Solution: make `StageMetadata.ModType` be `types.ModType` (`internal/domain/types/mods.go`) and validate on decode (reject unknown mod types).
- Add validation for `RunState` and `StageState`.
  - `RunState`/`StageState` are open string types with constants (`internal/mods/api/types.go:14`, `internal/mods/api/types.go:27`).
  - Solution: add `Validate()` methods so decoding can reject unknown states rather than silently mapping them later.

## Streamlining / Simplification

- Remove/merge unused state variants or map them consistently.
  - `StageStateQueued` exists (`internal/mods/api/types.go:18`) but `StageStatusFromStore` maps `store.JobStatusQueued` to `StageStatePending` (`internal/mods/api/status_conversion.go:26`), and `StageStatusToStore` maps `StageStateQueued` to `store.JobStatusCreated` (`internal/mods/api/status_conversion.go:81`).
  - Solution: either remove `queued` from the API surface or map it consistently in both directions (and add tests that assert the chosen mapping).
- Consolidate status conversion tables.
  - Conversion functions are repetitive switch statements (`internal/mods/api/status_conversion.go:24`, `internal/mods/api/status_conversion.go:52`).
  - Solution: express conversions as explicit maps + small helpers to reduce drift between forward/backward mappings and make “unknown” handling explicit.

## Likely Bugs / Risks

- Run success/failure ambiguity in `RunStatusFromStore`.
  - `store.RunStatusFinished` maps to `RunStateSucceeded` unconditionally (`internal/mods/api/status_conversion.go:57`), but “finished” does not mean “succeeded” if any jobs/repos failed.
  - Solution: derive API `RunState` from outcomes (e.g., inspect per-job/per-repo results or stats), or introduce an explicit run outcome field separate from lifecycle state.
- Weak spec validation boundary.
  - `RunSubmitRequest.Spec` is `json.RawMessage` (`internal/mods/api/types.go:50`) but the type does not require `json.Valid` or “must be object”.
  - Solution: validate spec shape at the server boundary (object-only if it will be merged/inspected) and cap request size per `roadmap/refactor/contracts.md` § "JSON Boundary Decoding (Server)".
