# CLI Mods Refactor Notes (`internal/cli/mods`)

- Mods API type hardening is tracked in `roadmap/refactor/mods-api.md`.

## Type Hardening

- Use domain newtypes for IDs in all Mods CLI commands.
  - Many commands use raw `string` identifiers:
    - `PullResolution.RunID`/`RepoID` (`internal/cli/mods/pull.go:30`).
    - `RunPullCommand.RunID` (`internal/cli/mods/pull.go:51`).
    - `ListRunRepoDiffsCommand.RepoID`, `DownloadDiffCommand.RepoID`, `DownloadDiffCommand.DiffID` (`internal/cli/mods/status.go:40`, `internal/cli/mods/status.go:98`).
    - Mod management + repos use `ModID string` and `RepoID string` (`internal/cli/mods/mod_management.go:184`, `internal/cli/mods/mod_repos.go:165`).
  - Solution:
    - Use `types.RunID` and `types.ModRepoID` for `run_id` / `repo_id` everywhere.
    - Use `types.ModID` for mod project IDs in responses.
    - If the path param accepts “id OR name”, introduce a distinct newtype (e.g. `types.ModRef`) to avoid accidentally treating names as IDs.
    - For UUID-backed resources (diff id), introduce a validated UUID newtype (e.g. `types.DiffID`) instead of raw strings.
- Use domain newtypes for VCS inputs.
  - `AddModRepoCommand` and `CreateModRunCommand` only check “non-empty” for `repo_url`/`base_ref`/`target_ref` (`internal/cli/mods/mod_repos.go:58`, `internal/cli/mods/mod_run.go:80`).
  - `SubmitCommand` and `CreateBatchCommand` validate via domain types, but only after trimming strings (`internal/cli/mods/submit.go:38`, `internal/cli/mods/batch.go:49`).
  - Solution: make request structs use `types.RepoURL` + `types.GitRef` directly (so validation happens at decode time) and validate *lists* (`RepoURLs`) item-by-item.
- Stop duplicating canonical payload structs (SSE + status).
  - `ArtifactsCommand` iterates `summary.Stages map[string]...` and treats keys as stage IDs (`internal/cli/mods/artifacts.go:70`).
  - Solution: once `modsapi.RunSummary.Stages` becomes `map[types.JobID]StageStatus` (see `roadmap/refactor/mods-api.md`), update CLI code to treat keys as `types.JobID`, not `string`.
- Decode timestamps as `time.Time`, not `string`, for correctness checks.
  - Mod and mod-repo summaries use `CreatedAt string` (`internal/cli/mods/mod_management.go:27`, `internal/cli/mods/mod_repos.go:25`).
  - Solution: use `time.Time` so JSON decoding enforces RFC3339 and call sites can compare/sort without string assumptions.

## Streamlining / Simplification

- Merged slice: CLI HTTP boundary behavior.
  - Implement once in a shared helper and reuse across all Mods CLI commands.
- Merged slice: gzip diff download streaming.
  - Implement one streaming gunzip helper (no “read all gz then gunzip all”) and reuse in Mods + Runs CLI.

## Likely Bugs / Risks

- Incorrect “is this an ID?” heuristic for mods.
  - `ResolveModByNameCommand` treats “UUID-like” strings as IDs (`internal/cli/mods/mod_management.go:430`), but `types.ModID` is NanoID-based (`internal/domain/types/ids.go:30`).
  - Solution: remove this heuristic and rely on an explicit server-side resolution contract (or always treat input as a `types.ModRef` and let the server resolve deterministically).
- URL path params not consistently escaped/validated.
  - Some commands escape ids (`url.PathEscape`), others pass raw (`internal/cli/mods/pull.go:72`, `internal/cli/mods/pull.go:167`).
  - Solution: validate newtyped identifiers to be URL-safe and consistently escape dynamic segments.
- Partial error-body handling.
  - Some paths cap error bodies (`decodeHTTPError`), while others attempt to decode `{error: ...}` without strictness and without a cap (`internal/cli/mods/submit.go:90`).
  - Merged slice: unify error decoding + caps once in a shared CLI HTTP helper.

## Suggested Minimal Slices

- Slice 1: Apply the shared CLI HTTP boundary helper (merged slice).
- Slice 2: Switch `run_id`/`repo_id`/`repo_url`/refs to domain newtypes across Mods CLI commands.
- Slice 3: Apply the shared streaming gunzip helper (merged slice).
