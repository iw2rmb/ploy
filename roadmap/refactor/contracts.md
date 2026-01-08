# Cross-Cutting Contracts (Refactor)

This file centralizes cross-cutting “contract” decisions that otherwise repeat across `roadmap/refactor/*`.

## IDs and Newtypes (`internal/domain/types`)

- Use domain newtypes for identifiers as early as possible (decode/validate once at the boundary), and keep them end-to-end:
  - `types.RunID`, `types.JobID`, `types.NodeID`, `types.ModID`, `types.SpecID`, `types.ModRepoID` (`internal/domain/types/ids.go`).
  - `types.RepoURL`, `types.GitRef`, `types.CommitSHA` (`internal/domain/types/vcs.go`).
- Do not keep raw `string` IDs in internal APIs after boundary decoding. Prefer typed helpers that return these types (path params, JSON bodies, config/env).
- Store/sqlc: map DB id columns and query args/returns to these types via sqlc overrides instead of `string`.
- Canonical domain response structs must not reintroduce raw `string` IDs:
  - Example: `types.RunSummary.ModID` and `types.RunSummary.SpecID` should be `types.ModID` / `types.SpecID` (not `string`) so server+CLI cannot drift (`internal/domain/types/runsummary.go`).
- Mods API types (`internal/mods/api`) must use these newtypes directly:
  - Request fields like `repo_url`/`base_ref`/`target_ref` should be `types.RepoURL`/`types.GitRef`, not raw strings.
  - Map keys that are semantically IDs (e.g., stages keyed by job id) should use `map[types.JobID]...` so JSON stays string-keyed but internal code is type-safe.
  - Repo-scoped endpoints should take `repo_id` as `types.ModRepoID` (not `string`) in internal APIs and CLI commands.
- CLI Mods command shapes should also be typed:
  - `pull` resolution response should use `types.RunID` / `types.ModRepoID` and `types.GitRef` for `repo_target_ref` (not raw strings) (`internal/cli/mods/pull.go`).
  - If an endpoint accepts “mod id OR name” in the path, use a distinct newtype (e.g. `types.ModRef`) so callers do not conflate it with `types.ModID`.

## Transfer Types (CLI + Server)

- Transfer identifiers and discriminators must be typed.
  - `slot_id` should be a `SlotID` newtype (non-empty, URL-safe).
  - `kind`/`stage` fields should be enums/newtypes with `Validate()` to prevent free-form strings.
  - `digest` should be a validated newtype (algorithm + value, e.g. `sha256:<hex>`), not a raw string.

## Streaming (SSE) Types

- SSE stream identifiers are run-scoped.
  - Use `types.RunID` as the stream identifier for run streams (not `string`) so invalid/blank IDs are rejected at the boundary.
- SSE resumption cursor must be typed and non-negative.
  - Add `types.EventID` (backed by `int64`) and validate `>= 0` at parse time before passing into hub/history selection.
  - CLI streaming (`internal/cli/stream`) should track this typed cursor for run streams and stringify only at the HTTP header boundary (`Last-Event-ID`).
- SSE event types must be a closed set.
  - Define an allow-list enum/newtype for event types (`log`, `retention`, `run`, `stage`, `done`) and validate at publish time.
- SSE payload structs must be canonical and type-safe.
  - Do not duplicate “log” payload structs across packages (e.g., CLI vs hub). Use one canonical struct (prefer `internal/stream.LogRecord`) so `node_id`/`job_id`/`mod_type`/`step_index` cannot drift.
  - `step_index` must be `types.StepIndex` (not `int`) and must satisfy `StepIndex.Valid()`.
  - `mod_type` must be `types.ModType` (not a free-form string).
  - Retention fields like `ttl` should be `types.Duration` (not ad-hoc strings).

## JSON Boundary Decoding (Server)

- Standardize request body decoding at API boundaries:
  - Enforce a request size cap (`http.MaxBytesReader`).
  - Use `json.Decoder.DisallowUnknownFields()` so contract drift fails fast.
  - Prefer `UseNumber()` only if you need to preserve numeric type/precision; otherwise parse into typed structs.
- Treat spec-like blobs as `json.RawMessage` at boundaries. If you must inspect/merge:
  - Only accept JSON objects for merge operations.
  - Reject invalid JSON and non-object JSON with a 400; never silently substitute `{}`.

## HTTP Boundary Decoding (CLI)

- CLI HTTP behavior must be standardized across `internal/cli/*`:
  - URL building must not drop `BaseURL.Path`:
    - Do not pass leading-slash segments to `(*url.URL).JoinPath` / `ResolveReference` (e.g. `"/v1/..."`).
    - Prefer `url.JoinPath(BaseURL.String(), "v1", ...)` (or `BaseURL.JoinPath("v1", ...)`).
  - Response decoding must fail fast on contract drift:
    - Use `json.Decoder.DisallowUnknownFields()` for JSON responses.
    - Cap response-body reads (including error bodies) with `io.LimitReader`.
  - Error shaping must be consistent:
    - Prefer a canonical `{ "error": "..." }` envelope when present; otherwise fall back to trimmed body text; otherwise `resp.Status`.
  - Long-running CLI operations must not use an infinite-timeout `http.Client`.

## StepIndex (Ordering Invariant)

- Use `types.StepIndex` (`internal/domain/types/ids.go`) end-to-end where a step ordering value exists (store rows, workflow graph, server events).
- Enforce invariants via `StepIndex.Valid()`:
  - Reject NaN/Inf.
  - Reject fractional values (must be integer-like).
- Do not cast `StepIndex` to `int` for sorting/serialization; that destroys ordering.
- Any generation of new step indices (healing/re-gate, etc.) must produce integer-like values only.
- Any sorting based on `StepIndex` must be deterministic:
  - Use a tie-breaker (`JobID`/node id) when `StepIndex` matches.
  - Treat duplicate IDs (graph nodes) as an error, not overwrite.

## Resource Units & Heartbeat

- Heartbeat is a strict, unit-explicit contract:
  - Prefer integer bytes for memory/disk (`mem_{free,total}_bytes`, `disk_{free,total}_bytes`).
  - Prefer integer millicores/millis for CPU (`cpu_{free,total}_millis`) and validate it fits target storage types.
- Avoid redundant/ambiguous identity fields in the heartbeat body:
  - Do not accept `node_id`/`timestamp` from the agent as authoritative when `{id}` is in the path; either remove them or enforce `node_id == {id}`.
- Update agent + server together and add contract tests.
