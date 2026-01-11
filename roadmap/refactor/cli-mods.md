# CLI Mods Refactor Slices (`internal/cli/mods`)

This file is a *roadmap slice plan* for refactoring Mods CLI clients under `internal/cli/mods/*`.

Prerequisite work that changes the shared wire types lives in `roadmap/refactor/mods-api.md`.

## Current State (Anchor at HEAD)

- `types.ModRef` already exists (`internal/domain/types/ids.go`) and is used by mod management commands (`internal/cli/mods/mod_management.go`).
- `ResolveModByNameCommand` does not use “UUID-like” heuristics; it always queries the server (`internal/cli/mods/mod_management.go`).
- Error-body caps are already centralized via `decodeHTTPError → httpx.WrapError` (`internal/cli/mods/batch.go`, `internal/cli/httpx/httpx.go`).
- Remaining issues at HEAD:
  - Several commands still use raw `string` identifiers (not domain newtypes) (`internal/cli/mods/pull.go`, `internal/cli/mods/status.go`, `internal/cli/mods/mod_repos.go`, `internal/cli/mods/mod_run.go`).
  - Diff downloads still “read all gz then gunzip all” (`internal/cli/mods/status.go`, `internal/cli/runs/diffs.go`).
  - URL construction/escaping is inconsistent; some code pre-escapes segments before `BaseURL.JoinPath(...)` (risk: double-escaping) (`internal/cli/mods/mod_repos.go`, `internal/cli/mods/mod_run.go`).
  - Several `created_at` fields are decoded as `string` instead of `time.Time` (`internal/cli/mods/mod_management.go`, `internal/cli/mods/mod_repos.go`, `internal/cli/mods/status.go`).

## Suggested Minimal Slices

### Slice 1: Streaming gunzip helper (Mods + Runs)

- What changes:
  - Replace “read all gzipped bytes into memory, then gunzip” with a streaming gunzip path.
- Where:
  - Introduce a shared helper under `internal/cli/httpx` (e.g. `GunzipToBytes(r io.Reader, limit int64) ([]byte, error)`).
  - Update `internal/cli/mods/status.go` and `internal/cli/runs/diffs.go` to stream-decompress directly from `resp.Body` with a size cap.
- Compatibility impact: none (internal refactor; wire format unchanged).
- Unchanged behavior:
  - Same endpoints and query params.
  - Same patch bytes returned to callers.

### Slice 2: Add a validated diff-id newtype

- What changes:
  - Introduce a UUID-backed identifier type (e.g. `types.DiffID`) instead of raw `string` diff IDs.
  - Use it in diff listing/download structs and CLI command parameters.
- Where:
  - Add new type in `internal/domain/types` (new file is fine; keep UUID validation local to this type).
  - Update `internal/cli/mods/status.go` and `internal/cli/runs/diffs.go` to use `types.DiffID` for:
    - list response IDs,
    - download request `diff_id` query param.
  - Update call sites (notably `cmd/ploy/pull_helpers.go`) to thread `types.DiffID` through.
- Compatibility impact: none (diff IDs are still transported as JSON strings and query params).
- Unchanged behavior:
  - Same server-generated diff ID strings; client now validates they are UUIDs.

### Slice 3: Mods CLI type hardening + URL construction cleanup

- What changes:
  - Convert Mods CLI command structs and response structs to domain newtypes:
    - `run_id` as `types.RunID`,
    - `repo_id` as `types.ModRepoID`,
    - mod project IDs as `types.ModID`,
    - “id-or-name” params stay as `types.ModRef` (already present).
  - Validate VCS inputs using domain types:
    - `repo_url` as `types.RepoURL`,
    - `base_ref`/`target_ref` as `types.GitRef`,
    - validate `RepoURLs` lists item-by-item.
  - Normalize URL construction:
    - stop pre-escaping segments passed to `BaseURL.JoinPath(...)`,
    - pick one consistent pattern across Mods CLI (either `url.JoinPath(base.String(), ..., url.PathEscape(seg))` or `base.JoinPath(...raw...)` with explicit validation that segments are URL-safe).
  - Decode `created_at` timestamps into `time.Time` where the server returns RFC3339.
- Where:
  - Primary files: `internal/cli/mods/pull.go`, `internal/cli/mods/status.go`, `internal/cli/mods/mod_repos.go`, `internal/cli/mods/mod_run.go`, `internal/cli/mods/mod_management.go`.
  - CLI entrypoints: `cmd/ploy/*` that construct these commands.
- Compatibility impact: none (wire format unchanged; internal types become strict).
- Unchanged behavior:
  - CLI flags/args remain strings; parsing converts to typed values and errors early on invalid inputs.

### Slice 4: Mods API prerequisite (then fix CLI artifacts staging types)

- What changes:
  - Apply the Mods API type changes that unblock CLI correctness:
    - `modsapi.RunSummary.Stages` becomes `map[types.JobID]StageStatus`.
    - (Optionally in the same slice) `modsapi.RunSubmitRequest` uses `types.RepoURL` and `types.GitRef`.
- Where:
  - Implemented per `roadmap/refactor/mods-api.md` in `internal/mods/api/types.go`.
  - Then update `internal/cli/mods/artifacts.go` to iterate typed keys (`types.JobID`) instead of `string`.
- Compatibility impact: none on the wire (JSON map keys remain strings); internal compiler-checked safety improves.
- Unchanged behavior:
  - `GET /v1/runs/{id}/status` payload shape remains the same JSON.
