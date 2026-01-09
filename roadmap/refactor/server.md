# Server Refactor Notes (`internal/server`)

- Workspace diff was clean at time of review (`git diff` empty); `git status` had untracked `roadmap/refactor/`.
- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md`.

## Type Hardening

- Standardize JSON decoding at API boundaries:
  - Many handlers decode via `json.NewDecoder(...).Decode(&req)` without `DisallowUnknownFields`, `UseNumber`, or a request size cap (e.g. `internal/server/handlers/nodes_heartbeat.go:29`, `internal/server/handlers/mods.go:193`).
  - Contrast with capped endpoints (e.g. `internal/server/handlers/nodes_logs.go:34`, `internal/server/handlers/runs_events.go:29`).
  - Solution: implement the standard JSON boundary rules in `roadmap/refactor/contracts.md` § "JSON Boundary Decoding (Server)".
- Switch API identifiers to `internal/domain/types` newtypes at boundaries (path params, query params, and JSON bodies):
  - Solution: follow `roadmap/refactor/contracts.md` § "IDs and Newtypes (`internal/domain/types`)".
- Reduce `map[string]any` at boundaries where practical:
  - “merge” helpers parse JSON to `map[string]any` then re-marshal; error paths often fall back to `{}` and silently drop input (`internal/server/handlers/spec_utils.go:29`, `internal/server/handlers/nodes_claim.go:221`).
  - Solution: follow `roadmap/refactor/contracts.md` § "JSON Boundary Decoding (Server)" (spec blobs must not silently coerce to `{}`).
- Fix heartbeat request/response contract (integer + explicit units; strict decoding):
  - Current server heartbeat request uses `float64` and truncation casts into integer DB columns (`internal/server/handlers/nodes_heartbeat.go:30`, `internal/server/handlers/nodes_heartbeat.go:66`), which makes overflow/truncation possible and prevents strict decoding.
  - Target: heartbeat request fields should be integers with explicit units, aligned with storage and the node listing response (`internal/store/schema.sql` nodes `*_bytes` / `*_millis`, `internal/server/handlers/nodes.go` JSON fields):
    - Prefer `mem_{free,total}_bytes` and `disk_{free,total}_bytes` as integer bytes (not `*_mb` floats).
    - Keep CPU as integer millicores/millis (`cpu_{free,total}_millis`) and validate it fits `int32`.
  - Enforce strict JSON decoding at the heartbeat boundary:
    - Apply a request body size cap (`http.MaxBytesReader`).
    - Use `DisallowUnknownFields`.
    - Validate invariants before DB write (non-negative; free <= total; fit-range for `int32`/`int64`).
  - Remove redundant/ambiguous fields from the heartbeat body (e.g. `node_id`, `timestamp` from `internal/nodeagent/heartbeat.go`) or explicitly accept them but enforce `node_id == {id}`; do not silently ignore drift.
  - Solution: follow `roadmap/refactor/contracts.md` § "Resource Units & Heartbeat".

## Simplifications

- Replace repo URL filtering’s N+1 scans with a store query (join/EXISTS):
  - Current handler paginates mods and lists repos per mod (`internal/server/handlers/mods.go:185`).
  - Solution: add a store query that filters mods by `repo_url` via a join/`EXISTS`, and have the handler call it directly.
- Simplify token revocation “no rows” detection:
  - Current code checks `sql.ErrNoRows`, `*pgconn.PgError` code, and string matching (`internal/server/auth/authorizer.go:322`).
  - Prefer one approach (`errors.Is(err, pgx.ErrNoRows)` or a single canonical sentinel).
  - Solution: standardize on `errors.Is(err, pgx.ErrNoRows)` and remove string/pg error-code matching.

## Likely Bugs / Risks

- Healing step index generation can violate the StepIndex invariant:
  - `domaintypes.StepIndex` must be finite (reject NaN/Inf) (`internal/domain/types/ids.go`).
  - New heal/re-gate indices are computed from `failedStepIndex` using fractional math (`gapSize*0.5`, `gapSize*0.75`) (`internal/server/handlers/nodes_complete_healing.go:221`).
  - Solution: follow `roadmap/refactor/contracts.md` § "StepIndex (Ordering Invariant)".
- Log enrichment truncates ordering:
  - Any `float -> int` cast of `job.StepIndex` would silently truncate and break ordering/cutoffs.
  - Ensure log enrichment publishes `types.StepIndex` end-to-end without truncation (`roadmap/refactor/scope.md`, `roadmap/refactor/contracts.md`).
- Config watcher reload context + debounce race:
  - Reload uses `context.Background()` (no cancellation/timeout), and `time.AfterFunc` can fire after stop (`internal/server/config/watcher.go:149`, `internal/server/config/watcher.go:176`).
  - Solution: thread a bounded context (timeout + cancellation) into reload, and ensure the debounce timer is stopped/drained on shutdown.
- Auth spawns one goroutine per request and ignores request cancellation:
  - `go a.updateTokenLastUsed(context.Background(), ...)` (`internal/server/auth/authorizer.go:290`).
  - Solution: use the request context (or a short timeout) and move updates behind a bounded worker/queue if it must be async.
- Metrics server stores a parent `context.Context` and `Reload` restarts with it:
  - If `parent` is canceled, restart can fail (`internal/server/metrics/server.go:31`, `internal/server/metrics/server.go:125`).
  - Solution: stop storing a long-lived parent context; pass a fresh context to each start/reload and explicitly manage shutdown via `Server.Close`.
- Scheduler ignores tasks added after start:
  - `AddTask` only appends; `Start` only spawns goroutines for the current slice (`internal/server/scheduler/scheduler.go:31`, `internal/server/scheduler/scheduler.go:52`).
  - Solution: either (a) forbid `AddTask` after `Start` (return error) or (b) make `Start` watch a channel so tasks added later are scheduled.
- Batch repo starter increments `Started` even when no job is scheduled:
  - `ScheduleNextJob` `pgx.ErrNoRows` is treated as non-fatal but still counts as `Started` (`internal/server/handlers/runs_batch_scheduler.go:106`).
  - Solution: only increment `Started` when a job was actually scheduled (distinct outcome vs “no rows”).
- Constructors `panic` on nil deps:
  - `createNodeLogsHandler` / `createRunLogHandler` panic if `eventsService` is nil (`internal/server/handlers/nodes_logs.go:32`, `internal/server/handlers/runs_events.go:27`).
  - Solution: return `(http.Handler, error)` from constructors and fail fast at server startup with a clear error.
