# CLI Runs Refactor Notes (`internal/cli/runs`)

- Cross-cutting contracts live in `roadmap/refactor/contracts.md` (IDs/newtypes, JSON decoding rules, SSE cursor/event types).

## Type Hardening

- Use domain ID newtypes for all identifiers in run CLI commands.
  - `RunID`/`JobID` are already typed (`internal/cli/runs/start.go:20`, `internal/cli/runs/follow.go:24`).
  - `RepoDiffsCommand.RepoID` is a raw `string` but is semantically `repo_id` (`mod_repos.id`), which already has a domain type: `types.ModRepoID` (`internal/domain/types/ids.go:41`).
  - Solution: switch `RepoDiffsCommand.RepoID` to `types.ModRepoID` and validate via `IsZero()` before building the URL.
- Avoid `string`-typed timestamps in CLI responses where ordering matters.
  - Diffs listing uses `CreatedAt string` (`internal/cli/runs/diffs.go:56`) and assumes the API returns “newest first” without verifying.
  - Solution: decode `created_at` as `time.Time` and sort locally when selecting “newest” (or make the API contract explicit and test it).
- Standardize JSON decoding strictness for CLI control-plane responses.
  - CLI decodes JSON via `json.NewDecoder(...).Decode(...)` without `DisallowUnknownFields` (`internal/cli/runs/start.go:65`, `internal/cli/runs/status.go:65`).
  - Merged slice: implement once per `roadmap/refactor/contracts.md` § "HTTP Boundary Decoding (CLI)" and reuse across CLI commands.

## Streamlining / Simplification

- Centralize request/response boilerplate.
  - `StartCommand` and `GetStatusCommand` repeat: input validation, endpoint building, status check, error body read, JSON decode.
  - Implement a shared helper per `roadmap/refactor/contracts.md` § "HTTP Boundary Decoding (CLI)" and reuse across CLI commands.

## Likely Bugs / Risks

- Unbounded error-body reads (merged slice).
  - Fix once per `roadmap/refactor/contracts.md` § "HTTP Boundary Decoding (CLI)".
- Ambiguous stop vs cancel semantics.
  - `StopCommand` posts to `/cancel` and returns a run summary, while `CancelCommand` also posts to `/cancel` but returns only “Cancellation requested” (`internal/cli/runs/stop.go:21`, `internal/cli/runs/cancel.go:20`).
  - Solution: keep **cancel** semantics only:
    - Keep `CancelCommand` as the single implementation (request cancellation; treat 202 Accepted as success; do not require returning a summary).
    - Remove `StopCommand` (no aliases).

## Suggested Minimal Slices

- Slice 1: Type `repo_id` as `types.ModRepoID` in `RepoDiffsCommand`.
- Slice 2: Apply the shared CLI HTTP boundary helper (merged slice).
- Slice 3: Apply the shared streaming gunzip helper (merged slice).
