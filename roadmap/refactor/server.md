# Server Refactor Notes (`internal/server`)

- Workspace diff was clean at time of review (`git diff` empty); `git status` had untracked `roadmap/refactor/`.

## Type Hardening

- Standardize JSON decoding at API boundaries:
  - Many handlers decode via `json.NewDecoder(...).Decode(&req)` without `DisallowUnknownFields`, `UseNumber`, or a request size cap (e.g. `internal/server/handlers/nodes_heartbeat.go:29`, `internal/server/handlers/mods.go:193`).
  - Contrast with capped endpoints (e.g. `internal/server/handlers/nodes_logs.go:34`, `internal/server/handlers/runs_events.go:29`).
- Reduce `map[string]any` at boundaries where practical:
  - “merge” helpers parse JSON to `map[string]any` then re-marshal; error paths often fall back to `{}` and silently drop input (`internal/server/handlers/spec_utils.go:29`, `internal/server/handlers/nodes_claim.go:221`).
- Make heartbeat resource values integer/typed units:
  - Heartbeat request uses `float64` and truncation casts (`internal/server/handlers/nodes_heartbeat.go:30`, `internal/server/handlers/nodes_heartbeat.go:66`).

## Simplifications

- Replace repo URL filtering’s N+1 scans with a store query (join/EXISTS):
  - Current handler paginates mods and lists repos per mod (`internal/server/handlers/mods.go:185`).
- Simplify token revocation “no rows” detection:
  - Current code checks `sql.ErrNoRows`, `*pgconn.PgError` code, and string matching (`internal/server/auth/authorizer.go:322`).
  - Prefer one approach (`errors.Is(err, pgx.ErrNoRows)` or a single canonical sentinel).

## Likely Bugs / Risks

- Healing step index generation can violate the StepIndex invariant:
  - `domaintypes.StepIndex` is float64 but requires integer-like values (`internal/domain/types/ids.go:43`).
  - For `re_gate` failures, `baseGateIndex` can become a non-integer if it was derived from a prior healing `StepIndex` (`internal/server/handlers/nodes_complete_healing.go:116`).
  - New heal/re-gate indices are computed from `failedStepIndex` using fractional math (`gapSize*0.5`, `gapSize*0.75`) (`internal/server/handlers/nodes_complete_healing.go:221`).
- Log enrichment truncates ordering:
  - `job.StepIndex` (float64) is cast to `int` for stream records (`internal/server/events/service.go:296`).
- Spec merge helpers can destroy non-object JSON:
  - If spec is not an object, unmarshal fails and code falls back to `{}`; then it writes only injected keys (`internal/server/handlers/nodes_claim.go:221`, `internal/server/handlers/spec_utils.go:29`).
- Config watcher reload context + debounce race:
  - Reload uses `context.Background()` (no cancellation/timeout), and `time.AfterFunc` can fire after stop (`internal/server/config/watcher.go:149`, `internal/server/config/watcher.go:176`).
- Auth spawns one goroutine per request and ignores request cancellation:
  - `go a.updateTokenLastUsed(context.Background(), ...)` (`internal/server/auth/authorizer.go:290`).
- Metrics server stores a parent `context.Context` and `Reload` restarts with it:
  - If `parent` is canceled, restart can fail (`internal/server/metrics/server.go:31`, `internal/server/metrics/server.go:125`).
- Scheduler ignores tasks added after start:
  - `AddTask` only appends; `Start` only spawns goroutines for the current slice (`internal/server/scheduler/scheduler.go:31`, `internal/server/scheduler/scheduler.go:52`).
- Batch repo starter increments `Started` even when no job is scheduled:
  - `ScheduleNextJob` `pgx.ErrNoRows` is treated as non-fatal but still counts as `Started` (`internal/server/handlers/runs_batch_scheduler.go:106`).
- Constructors `panic` on nil deps:
  - `createNodeLogsHandler` / `createRunLogHandler` panic if `eventsService` is nil (`internal/server/handlers/nodes_logs.go:32`, `internal/server/handlers/runs_events.go:27`).
