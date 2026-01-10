# Refactor: strict contracts + typed boundaries (`roadmap/refactor`)

Scope: Implement the full refactor described in `roadmap/refactor/scope.md` and the referenced notes under `roadmap/refactor/`. Make API + internal contracts strict and type-safe by using `internal/domain/types` end-to-end. Eliminate drift between server/store/stream/CLI. No backward-compat layers. Remove replaced code.

Documentation:
- `roadmap/refactor/scope.md`
- `roadmap/refactor/contracts.md`
- `roadmap/refactor/store.md`
- `roadmap/refactor/server.md`
- `roadmap/refactor/stream.md`
- `roadmap/refactor/workflow.md`
- `roadmap/refactor/mods-api.md`
- `roadmap/refactor/worker.md`
- `roadmap/refactor/cli-stream.md`
- `roadmap/refactor/cli-logs.md`
- `roadmap/refactor/cli-runs.md`
- `roadmap/refactor/cli-mods.md`
- `roadmap/refactor/cli-trasnfer.md`

Legend: [ ] todo, [x] done.

## Contracts (domain types + invariants)
- [x] Add typed SSE cursor (`types.EventID`) — stop passing unvalidated `int64` cursors across layers
  - Repository: ploy
  - Component: `internal/domain/types`
  - Scope: Add `type EventID int64` with `Valid() bool` (`>=0`), plus text/json marshal helpers; use it at boundaries (header parsing, hub subscribe) instead of raw `int64` (`internal/domain/types/ids.go`, `internal/stream/hub.go`, `internal/cli/stream/*`).
  - Snippets:
    - ```go
      type EventID int64
      func (v EventID) Valid() bool { return v >= 0 }
      ```
  - Tests: `go test ./internal/domain/types -run TestEventID` — cursor rejects negatives and round-trips text/json

- [x] Define a closed SSE event-type enum — prevent drift and “random string” publishes
  - Repository: ploy
  - Component: `internal/domain/types` + `internal/stream`
  - Scope: Add `types.SSEEventType` (allow-list: `log`, `retention`, `run`, `stage`, `done`) and validate at publish time; remove ad-hoc `string` event types (`roadmap/refactor/contracts.md`, `internal/stream/hub.go`, `internal/stream/http.go`).
  - Snippets:
    - ```go
      type SSEEventType string
      func (v SSEEventType) Validate() error { /* switch allow-list */ }
      ```
  - Tests: `go test ./internal/stream -run TestHubRejectsUnknownEventType` — publish fails for unknown types

- [x] Implement the canonical SSE/log payload contract end-to-end — eliminate duplicated structs and StepIndex truncation
  - Repository: ploy
  - Component: `internal/stream` + `internal/server` + `internal/cli/*`
  - Scope: Make `internal/stream.LogRecord` the single canonical “log” payload type and ensure it uses domain types (`types.NodeID`, `types.JobID`, `types.ModType`, `types.StepIndex`); switch all publishers/decoders to it; remove duplicate log payload structs (e.g. `internal/cli/logs.LogRecord`) and remove all lossy `float64 -> int` casts in publish paths (`internal/server/events/service.go`) (`roadmap/refactor/scope.md`, `roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`, `roadmap/refactor/stream.md`, `roadmap/refactor/cli-logs.md`).
  - Snippets:
    - ```go
      type LogRecord struct {
        NodeID   types.NodeID   `json:"node_id,omitempty"`
        JobID    types.JobID    `json:"job_id,omitempty"`
        ModType  types.ModType  `json:"mod_type,omitempty"`
        StepIndex types.StepIndex `json:"step_index,omitempty"`
      }
      ```
  - Tests: `go test ./internal/stream ./internal/server/... ./internal/cli/... -run TestLogRecord` — publishers/decoders compile and preserve typed fields

- [x] Use `types.RunID` as the run stream identifier everywhere — reject invalid/blank run IDs at the boundary
  - Repository: ploy
  - Component: `internal/stream` + `internal/server` + `internal/cli/stream`
  - Scope: Replace stream ID `string` parameters with `types.RunID` in hub/server/cli APIs; stringify only at HTTP boundaries (`Last-Event-ID`, path params) (`roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      func (h *Hub) PublishLog(ctx context.Context, runID types.RunID, rec LogRecord) error
      ```
  - Tests: `go test ./... -run TestRunIDRejectedAtStreamBoundary` — blank/whitespace run IDs fail before publish/subscribe

- [x] Make `types.RunSummary` ID fields typed — prevent server/CLI from reintroducing raw strings
  - Repository: ploy
  - Component: `internal/domain/types`
  - Scope: Change `internal/domain/types/runsummary.go` fields `ModID` and `SpecID` to `types.ModID` / `types.SpecID`; update all encoders/decoders and OpenAPI when implemented (`docs/api/OpenAPI.yaml`).
  - Snippets:
    - ```go
      ModID  ModID  `json:"mod_id"`
      SpecID SpecID `json:"spec_id"`
      ```
  - Tests: `go test ./... -run TestRunSummaryJSON` — JSON decode rejects empty IDs; callers compile with typed fields

- [x] Introduce “ref-or-id” type for mods (`types.ModRef`) — stop conflating IDs with names
  - Repository: ploy
  - Component: `internal/domain/types` + `internal/cli/mods` + server mods handlers
  - Scope: Add `types.ModRef` and use it for endpoints that accept “mod id OR name”; remove any “is this an ID?” heuristics in CLI (`roadmap/refactor/contracts.md`, `roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      type ModRef string // validate non-empty + URL-safe
      ```
  - Tests: `go test ./internal/cli/mods -run TestResolveModByNameNoHeuristic` — CLI does not special-case “UUID-like” inputs

- [x] Add strict transfer typing (`SlotID`, `TransferKind`, `TransferStage`, `Digest`) — fail fast on invalid transfer requests
  - Repository: ploy
  - Component: `internal/domain/types` + `internal/cli/transfer`
  - Scope: Add newtypes/enums with `Validate()` (including digest format like `sha256:<hex>`), and use them in `internal/cli/transfer/client.go` request/response structs (see `roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      type Digest string
      func (v Digest) Validate() error { /* sha256:<hex> */ }
      ```
  - Tests: `go test ./internal/cli/transfer -run TestDigestValidate` — invalid digests reject before HTTP

- [x] Consolidate StepIndex invariant checks at boundaries — remove lossy `float64 -> int` casts
  - Repository: ploy
  - Component: `internal/domain/types` + server + mods api + stream
  - Scope: Ensure all boundary parsing validates `types.StepIndex.Valid()` and all payloads use `types.StepIndex` (not `int`); remove any `int(job.StepIndex)` casts (`roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`, `roadmap/refactor/mods-api.md`).
  - Snippets:
    - ```go
      if !stepIndex.Valid() { return fmt.Errorf("invalid step_index") }
      ```
  - Tests: `go test ./... -run TestStepIndexNoTruncation` — fractional step indices round-trip without truncation

## Store (migrations + typed sqlc + deterministic ordering)
- [x] Set Postgres `search_path` in the pool config — stop correctness depending on DSN formatting
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Set `RuntimeParams["search_path"] = "ploy,public"` when building the pgx pool (`internal/store/store.go`) (see `roadmap/refactor/store.md`).
  - Snippets:
    - ```go
      cfg.ConnConfig.RuntimeParams["search_path"] = "ploy,public"
      ```
  - Tests: `go test ./internal/store -run TestConnectSearchPath` — unqualified queries work without DSN `search_path`

- [x] Implement tracked migrations (Option A only) — make schema application deterministic
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Add `ploy.schema_version` to `internal/store/schema.sql`; update `RunMigrations` to use `execMigrationSQL` and record versions; keep/update `internal/store/versioning.go` (`roadmap/refactor/store.md`).
  - Snippets:
    - ```sql
      CREATE TABLE IF NOT EXISTS ploy.schema_version (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL);
      ```
  - Tests: `go test ./internal/store -run TestRunMigrations` — schema_version table exists and versions are recorded

- [x] Add sqlc overrides for domain IDs and StepIndex — stop `string`/`float64` leakage from the store layer
  - Repository: ploy
  - Component: `internal/store` + `sqlc.yaml`
  - Scope: Add sqlc overrides mapping id columns and args/returns to `internal/domain/types` newtypes; map `jobs.step_index` to `types.StepIndex` (`roadmap/refactor/store.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```yaml
      overrides:
        - column: "ploy.jobs.step_index"
          go_type: "github.com/iw2rmb/ploy/internal/domain/types.StepIndex"
      ```
  - Tests: `go test ./internal/store -run TestSQLCOverridesCompile` — generated code compiles and returns typed IDs

- [x] Require non-null node id for job claiming — prevent `jobs.node_id = NULL` “Running” rows
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Replace `ClaimJob(ctx, nodeID *string)` with a typed non-null signature (e.g. `ClaimJob(ctx, nodeID types.NodeID)`); enforce non-null in SQL (`internal/store/querier.go`, `internal/store/queries/jobs.sql`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```go
      func (q *Queries) ClaimJob(ctx context.Context, nodeID domaintypes.NodeID) (...)
      ```
  - Tests: `go test ./internal/store -run TestClaimJobRequiresNodeID` — cannot claim with empty node id

- [x] Make job claiming ordering deterministic and scoped — stop global step_index-only ordering
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Update `ClaimJob` SQL ordering to include a tie-breaker (`ORDER BY step_index ASC, id ASC`) and confirm ordering scope is correct (per run/per repo) (`internal/store/queries/jobs.sql`) (`roadmap/refactor/store.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```sql
      ORDER BY j.step_index ASC, j.id ASC
      ```
  - Tests: `go test ./internal/store -run TestClaimJobOrderingDeterministic` — ties resolve by stable secondary key

- [x] Make scheduling atomic (select + update) — stop scheduler races
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Convert `ScheduleNextJob` to select with `FOR UPDATE SKIP LOCKED` and update with a status predicate; do not update rows that changed state concurrently (`internal/store/queries/jobs.sql`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```sql
      ... FOR UPDATE SKIP LOCKED
      ```
  - Tests: `go test ./internal/store -run TestScheduleNextJobNoRace` — concurrent schedulers do not double-start a job

- [x] Fix `duration_ms` NULL writes — align transitions with `NOT NULL` schema
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Update `CompleteJob`/duration computations to handle missing `started_at`; set `started_at` on transition to Running and compute duration defensively (`internal/store/queries/jobs.sql`, `internal/store/schema.sql`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```sql
      COALESCE(EXTRACT(EPOCH FROM (now() - started_at)) * 1000, 0)::BIGINT
      ```
  - Tests: `go test ./internal/store -run TestCompleteJobDurationNeverNull` — duration_ms always non-null

- [x] Reject unknown enum values at scan-time — fail fast on schema/code drift
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Add allow-lists in `JobStatus.Scan` / `RunStatus.Scan` and return an error on unknown string values (`internal/store/models.go`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```go
      default: return fmt.Errorf("unknown JobStatus %q", s)
      ```
  - Tests: `go test ./internal/store -run TestJobStatusScanRejectsUnknown` — Scan errors on unexpected values

- [x] Ensure `ClaimJob` locks only the intended rows — avoid unnecessary `runs` row locking
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Update `ClaimJob` query to avoid locking unrelated rows when using `FOR UPDATE SKIP LOCKED` (e.g., `FOR UPDATE OF jobs`); verify the lock scope matches the intended concurrency model (`internal/store/queries/jobs.sql`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```sql
      ... FOR UPDATE OF j SKIP LOCKED
      ```
  - Tests: `go test ./internal/store -run TestClaimJobLocksJobOnly` — concurrent claim does not block on unrelated rows

- [x] Validate JSON before writing JSONB columns — prevent storing invalid `[]byte` blobs
  - Repository: ploy
  - Component: `internal/server` + `internal/workflow` + `internal/store`
  - Scope: Before insert/update of JSONB columns (`jobs.meta`, `runs.stats`, `specs.spec`, `diffs.summary`), validate `json.Valid`; use `json.RawMessage` in boundary structs where possible (`roadmap/refactor/store.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      if len(raw) > 0 && !json.Valid(raw) { return fmt.Errorf("invalid JSON") }
      ```
  - Tests: `go test ./... -run TestRejectsInvalidJSONBPayloads` — invalid JSON does not reach the store

- [x] Apply deterministic tie-breakers to list queries — stop nondeterministic ordering on ties
  - Repository: ploy
  - Component: `internal/store`
  - Scope: Add stable secondary ordering (`id`, `created_at`, etc.) to all list queries ordering by non-unique columns (including any ordered by `step_index`) (`internal/store/queries/*.sql`) (`roadmap/refactor/store.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```sql
      ORDER BY created_at DESC, id DESC
      ```
  - Tests: `go test ./internal/store -run TestListQueriesDeterministicOrder` — tie cases have stable ordering

- [x] Add `List*Meta` store queries for blob-heavy endpoints — avoid `SELECT *` on lists
  - Repository: ploy
  - Component: `internal/store` + server handlers
  - Scope: Add “meta list” queries that return ids + timestamps + small fields; keep existing `Get*` for blob fetch (`internal/store/queries/logs.sql`, `internal/store/queries/diffs.sql`, `internal/store/queries/artifact_bundles.sql`, `internal/store/queries/events.sql`) (`roadmap/refactor/store.md`).
  - Snippets:
    - ```sql
      SELECT id, created_at FROM ploy.logs WHERE ... ORDER BY created_at DESC, id DESC
      ```
  - Tests: `go test ./internal/store -run TestListMetaQueriesDoNotReturnBlobs` — list paths do not scan large blob columns

## Server (strict JSON boundaries + heartbeat + StepIndex correctness)
- [x] Add a shared strict JSON decoder helper — enforce caps + `DisallowUnknownFields` everywhere
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Implement a helper used by all handlers to cap body size (`http.MaxBytesReader`) and call `json.Decoder.DisallowUnknownFields()`; convert handlers that do raw `Decode` (`roadmap/refactor/server.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
      dec := json.NewDecoder(r.Body); dec.DisallowUnknownFields()
      ```
  - Tests: `go test ./internal/server/handlers -run TestDecodeRejectsUnknownFields` — unknown JSON fields return 400

- [x] Switch server API identifiers to domain newtypes at boundaries — decode/validate once
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Parse path/query params and body IDs into `internal/domain/types` at handler boundaries; do not pass raw strings internally (`roadmap/refactor/server.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      runID, err := domaintypes.ParseRunIDParam(r, "run_id")
      ```
  - Tests: `go test ./internal/server/handlers -run TestPathParamsUseDomainTypes` — invalid/blank IDs return 400 before store calls

- [x] Make “merge spec JSON” reject invalid/non-object inputs — stop silent `{}` substitution
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Update merge helpers to treat spec blobs as `json.RawMessage`; require `json.Valid` and object-only when merging; return 400 on invalid/non-object (`internal/server/handlers/spec_utils.go`, `internal/server/handlers/nodes_claim.go`) (`roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      if len(raw) > 0 && !json.Valid(raw) { return badRequest(...) }
      ```
  - Tests: `go test ./internal/server/handlers -run TestMergeRejectsNonObject` — arrays/strings/invalid JSON rejected

- [ ] Introduce integer resource unit types — unblock strict heartbeat + worker resource accounting
  - Repository: ploy
  - Component: `internal/domain/types` + `internal/server` + `internal/worker`
  - Scope: Introduce unitful integer types (bytes, millis, etc.) and thread them through worker snapshot/resource producers and server boundaries before the wire/schema flip; delete float64 unitful fields where possible (`internal/domain/types/resources.go`, worker lifecycle/resource code, server heartbeat validation helpers) (`roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`, `roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      type Bytes int64
      ```
  - Tests: `go test ./internal/domain/types ./internal/worker/... -run TestResourceUnitsAreIntegers` — unitful quantities are integers and validated

- [ ] Update heartbeat contract + schema to integer + unit-explicit fields — remove float truncation risk
  - Repository: ploy
  - Component: `internal/server` + `internal/nodeagent` + `internal/store`
  - Scope: Replace float/MB fields with integer bytes/millis fields; remove or enforce redundant `node_id` in heartbeat body; validate invariants and fit-range before DB writes; update nodes schema fields (`internal/server/handlers/nodes_heartbeat.go`, `internal/nodeagent/heartbeat.go`, nodes schema fields in `internal/store/schema.sql`) (`roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      type HeartbeatRequest struct { MemFreeBytes int64 `json:"mem_free_bytes"` }
      ```
  - Tests: `go test ./internal/server/handlers -run TestHeartbeatStrictUnits` — floats/unknown fields rejected; negative values rejected

- [x] Enforce no-truncation log enrichment — preserve fractional StepIndex end-to-end
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Publish typed `types.StepIndex` into the canonical `internal/stream.LogRecord` and ensure log enrichment never truncates it (`roadmap/refactor/server.md`, `roadmap/refactor/scope.md`).
  - Snippets:
    - ```go
      rec.StepIndex = job.StepIndex // typed StepIndex, no cast
      ```
  - Tests: `go test ./internal/server/... -run TestEventPublishPreservesStepIndex` — StepIndex values round-trip without truncation

- [ ] Replace mods “repo_url filter” N+1 with a store query — reduce load and simplify handler logic
  - Repository: ploy
  - Component: `internal/server` + `internal/store`
  - Scope: Add a store query that filters mods by `repo_url` via JOIN/EXISTS; update `internal/server/handlers/mods.go` to call it (see `roadmap/refactor/server.md`).
  - Snippets:
    - ```sql
      WHERE EXISTS (SELECT 1 FROM ploy.mod_repos r WHERE r.mod_id = m.id AND r.repo_url = $1)
      ```
  - Tests: `go test ./internal/server/handlers -run TestModsListRepoURLFilterUsesStoreQuery` — handler returns correct filtered results

- [ ] Harden token authorizer side effects — idempotent revocation + bounded last-used updates
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Replace mixed `sql.ErrNoRows`/code/string checks with a single `errors.Is(err, pgx.ErrNoRows)` path and stop spawning unbounded `updateTokenLastUsed` goroutines; honor request cancellation or a short timeout (`internal/server/auth/authorizer.go`) (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      if errors.Is(err, pgx.ErrNoRows) { return nil }
      ```
  - Tests: `go test ./internal/server/auth -run TestRevokeTokenNoRowsIsNotError; go test ./internal/server/auth -run TestUpdateTokenLastUsedRespectsContext` — revocation is idempotent; update cancels promptly

- [ ] Fix config watcher debounce/reload lifetime — stop timers firing after shutdown
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Thread a bounded context into reload; stop/drain debounce timers on shutdown; avoid `context.Background()` reloads (`internal/server/config/watcher.go`) (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      ctx, cancel := context.WithTimeout(parent, 5*time.Second); defer cancel()
      ```
  - Tests: `go test ./internal/server/config -run TestWatcherStopCancelsDebounce` — no late reload after Stop

- [ ] Fix metrics server reload context handling — stop storing a canceled parent context
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Stop storing a long-lived parent ctx inside the metrics server; pass fresh contexts to start/reload and manage shutdown via server close (`internal/server/metrics/server.go`) (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      func (s *Server) Reload(ctx context.Context, cfg Config) error { ... }
      ```
  - Tests: `go test ./internal/server/metrics -run TestMetricsReloadAfterParentCancel` — reload works after prior ctx cancellation

- [ ] Define scheduler AddTask-after-Start behavior — forbid or support; do not silently ignore
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Choose policy: (a) return error if `AddTask` after `Start`, or (b) schedule tasks via a channel; update `internal/server/scheduler/scheduler.go` and tests (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      if s.started { return errors.New("cannot AddTask after Start") }
      ```
  - Tests: `go test ./internal/server/scheduler -run TestAddTaskAfterStartPolicy` — behavior is explicit and tested

- [ ] Fix batch repo starter “Started” accounting — only increment when a job is scheduled
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Update `internal/server/handlers/runs_batch_scheduler.go` to increment `Started` only on successful schedule (not on `pgx.ErrNoRows`) (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      if scheduled { result.Started++ }
      ```
  - Tests: `go test ./internal/server/handlers -run TestBatchSchedulerStartedCount` — no-rows does not increment Started

- [ ] Remove handler-constructor panics — fail fast at startup with explicit errors
  - Repository: ploy
  - Component: `internal/server`
  - Scope: Replace constructor `panic` on nil deps with `(http.Handler, error)` and handle errors at startup (`internal/server/handlers/nodes_logs.go`, `internal/server/handlers/runs_events.go`) (`roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      if events == nil { return nil, errors.New("events service required") }
      ```
  - Tests: `go test ./internal/server/handlers -run TestHandlerConstructorNilDeps` — constructors return errors, no panics

## Stream (hub safety + SSE framing + retention)
- [ ] Harden hub publish/subscribe safety — reject blank IDs + make send/close race-free
  - Repository: ploy
  - Component: `internal/stream`
  - Scope: Change `Hub.publish`/`Ensure` behavior to reject blank IDs and return an error; ensure no goroutine can send to a closed subscriber channel; stop closing in `subscriber.send` and only close after removal under lock; make send safe against concurrent drop/finish (`internal/stream/hub.go`) (`roadmap/refactor/stream.md`).
  - Snippets:
    - ```go
      if strings.TrimSpace(streamID) == "" { return errors.New("stream id required") }
      ```
  - Tests: `go test ./internal/stream -run TestPublishBlankStreamID; go test ./internal/stream -run TestConcurrentPublishDropNoPanic` — blank IDs error; stress publish/drop without panics

- [ ] Unify SSE serve path + remove framing allocations — keep one codepath, write bytes
  - Repository: ploy
  - Component: `internal/stream`
  - Scope: Keep one implementation with an optional filter function; delete the other and its tests; update framing to split by `\n` at byte level and write bytes directly; keep fuzz coverage (`internal/stream/http.go`, `internal/stream/http_fuzz_test.go`) (`roadmap/refactor/stream.md`).
  - Snippets:
    - ```go
      func Serve(w http.ResponseWriter, r *http.Request, sub Subscription, filter func(Event) bool)
      ```
  - Tests: `go test ./internal/stream -run TestServeFiltered; go test ./internal/stream -run FuzzWriteEventFrame` — serve still works; fuzz still passes with non-UTF8 data

- [ ] Make history retention O(1) — replace slice-copy truncation with ring buffer
  - Repository: ploy
  - Component: `internal/stream`
  - Scope: Replace `publish` truncation logic with a ring buffer (or moving start index) so steady-state doesn’t allocate/copy; keep IDs monotonic (`internal/stream/hub.go`) (`roadmap/refactor/stream.md`).
  - Snippets:
    - ```go
      // store history in circular buffer; compute logical index via (head+i)%cap
      ```
  - Tests: `go test ./internal/stream -run TestHistoryRetention` — resumption still returns the correct tail events

- [ ] Use binary search for `historyAfter` — avoid linear scans on subscribe
  - Repository: ploy
  - Component: `internal/stream`
  - Scope: Since `Event.ID` is monotonic, compute start offset via binary search; keep correctness for edge cases (`internal/stream/hub.go`) (`roadmap/refactor/stream.md`).
  - Snippets:
    - ```go
      i := sort.Search(len(hist), func(i int) bool { return hist[i].ID > since })
      ```
  - Tests: `go test ./internal/stream -run TestHistoryAfterBinarySearch` — returns same results as prior implementation

## Workflow (spec parsing + deterministic graph + runtime correctness)
- [ ] Tighten mods spec numeric/ordering semantics — reject mod_index + reject fractional ints + preserve retries:0
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Delete parse/serialize support for `mod_index` and return a validation error when present; replace `int(float64)` casts with `types.IntFromAny` / `types.Int64FromAny` and surface field-path errors; preserve explicit `retries: 0` vs unset (e.g., pointer field or custom marshal/unmarshal) (`internal/workflow/contracts/mods_spec.go`) (`roadmap/refactor/workflow.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      type Retries struct{ Value *int }
      ```
  - Tests: `go test ./internal/workflow/... -run TestSpecRejectsModIndex; go test ./internal/workflow/... -run TestSpecRejectsFractionalIntFields; go test ./internal/workflow/... -run TestRetriesZeroRoundTrip` — reject mod_index; reject fractional ints; preserve explicit zero

- [ ] Type server-injected IDs in specs — stop “job_id as string” drift
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Change spec-injected fields like `job_id` to `types.JobID` (not `string`) and validate on parse; update any helpers that accept `runID string` (e.g., `SubjectsForRun`) to take `types.RunID` (`internal/workflow/contracts/*`) (`roadmap/refactor/workflow.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      func SubjectsForRun(runID domaintypes.RunID) []Subject
      ```
  - Tests: `go test ./internal/workflow/... -run TestInjectedIDsAreTyped` — invalid injected IDs fail validation

- [ ] Replace “stringly typed” workflow enums and maps — reduce `map[string]any` at boundaries
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Introduce enums/newtypes for fields like `BuildGateLogFinding.Severity`; replace `BuildMeta.Metrics map[string]interface{}` with a typed struct or `json.RawMessage` with validation (`internal/workflow/contracts/build_gate_metadata.go`, `internal/workflow/contracts/job_meta.go`) (`roadmap/refactor/workflow.md`).
  - Snippets:
    - ```go
      type Severity string
      const SeverityError Severity = "error"
      ```
  - Tests: `go test ./internal/workflow/... -run TestWorkflowEnumsValidate` — unknown enum values reject

- [ ] Fix diff path normalization — rewrite only diff header/path lines
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Replace full-body `strings.ReplaceAll` with line-based rewriting for `diff --git`, `---`, `+++` headers only (`internal/workflow/runtime/step/stub.go`) (`roadmap/refactor/workflow.md`).
  - Snippets:
    - ```go
      if strings.HasPrefix(line, "diff --git ") { /* rewrite paths */ }
      ```
  - Tests: `go test ./internal/workflow/... -run TestNormalizeDiffPathsDoesNotRewriteHunks` — hunk contents unchanged

- [ ] Make workflow graph ordering deterministic — add tie-breaker; reject duplicate IDs
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Sort by `StepIndex` then `JobID` (or equivalent stable key); make `AddNode` error on duplicate IDs instead of overwrite (`internal/workflow/graph/types.go`) (`roadmap/refactor/workflow.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      if _, ok := g.nodes[id]; ok { return fmt.Errorf("duplicate node %q", id) }
      ```
  - Tests: `go test ./internal/workflow/... -run TestGraphDeterministicOrderAndNoOverwrite` — stable sort + duplicates error

- [ ] Implement container input mount modes for all inputs — align runtime with manifest semantics
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Mount all `Inputs` and honor per-input `Mode` (RO/RW); add tests asserting mount flags for multiple inputs (`internal/workflow/runtime/step/container_spec.go`) (`roadmap/refactor/workflow.md`).
  - Snippets:
    - ```go
      readonly := input.Mode == StepInputModeRO
      ```
  - Tests: `go test ./internal/workflow/... -run TestContainerMountsAllInputsWithMode` — mounts match the manifest

- [ ] Fix docker wait channel handling — handle closed channels explicitly
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: In docker wait select, read `err, ok := <-ch` and prioritize result channel once ready (`internal/workflow/runtime/step/container_docker.go`) (`roadmap/refactor/workflow.md`).
  - Snippets:
    - ```go
      err, ok := <-waitErrCh; if !ok { err = nil }
      ```
  - Tests: `go test ./internal/workflow/... -run TestDockerWaitHandlesClosedChannel` — no “wait interrupted” on normal close

- [ ] Make gate pass logic “all checks passed” — stop assuming `StaticChecks[0]` is authoritative
  - Repository: ploy
  - Component: `internal/workflow`
  - Scope: Treat gate pass as `all(Passed)` and define explicit behavior for empty checks (pass/fail); update logic and tests (`internal/workflow/runtime/step/stub.go`) (`roadmap/refactor/workflow.md`).
  - Snippets:
    - ```go
      passed := len(checks) > 0 && slices.All(checks, func(c Check) bool { return c.Passed })
      ```
  - Tests: `go test ./internal/workflow/... -run TestGatePassAllChecks` — multiple checks obey all() semantics

## Mods API (`internal/mods/api`) (typed shapes + consistent state mapping)
- [ ] Type submit request VCS fields — validate on JSON decode, not after-the-fact
  - Repository: ploy
  - Component: `internal/mods/api`
  - Scope: Change `RunSubmitRequest` fields to `types.RepoURL` / `types.GitRef`; update CLI/server callers and tests (`internal/mods/api/types.go`) (`roadmap/refactor/mods-api.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      RepoURL types.RepoURL `json:"repo_url"`
      ```
  - Tests: `go test ./internal/mods/api -run TestRunSubmitRequestStrictTypes` — invalid repo_url/ref rejected by decode

- [ ] Type the stages map key as `types.JobID` — eliminate stringly-typed stage IDs
  - Repository: ploy
  - Component: `internal/mods/api`
  - Scope: Change `RunSummary.Stages` to `map[types.JobID]StageStatus`; update encode/decode and all call sites (`internal/mods/api/types.go`) (`roadmap/refactor/mods-api.md`).
  - Snippets:
    - ```go
      Stages map[types.JobID]StageStatus `json:"stages"`
      ```
  - Tests: `go test ./internal/mods/api -run TestRunSummaryStagesMapKeyTextMarshaling` — JSON keys round-trip

- [ ] Fix `StageStatus.StepIndex` type — use `types.StepIndex` end-to-end
  - Repository: ploy
  - Component: `internal/mods/api` + server handlers
  - Scope: Change `StageStatus.StepIndex` to `types.StepIndex`; remove server truncation casts (`internal/server/handlers/mods_ticket.go`); validate `StepIndex.Valid()` at boundaries (`roadmap/refactor/mods-api.md`, `roadmap/refactor/server.md`).
  - Snippets:
    - ```go
      StepIndex types.StepIndex `json:"step_index"`
      ```
  - Tests: `go test ./... -run TestModsTicketDoesNotTruncateStepIndex` — step_index preserves ordering and rejects invalid values

- [ ] Type `StageMetadata.ModType` and validate states — enforce closed set at decode time
  - Repository: ploy
  - Component: `internal/mods/api`
  - Scope: Change `StageMetadata.ModType` to `types.ModType` and validate it; add `Validate()` for `RunState`/`StageState` and enforce in decode paths (`internal/mods/api/types.go`) (`roadmap/refactor/mods-api.md`).
  - Snippets:
    - ```go
      func (s StageState) Validate() error { /* allow-list */ }
      ```
  - Tests: `go test ./internal/mods/api -run TestModsAPIRejectsUnknownStates` — unknown states fail validation

- [ ] Make status/state/outcome mappings explicit — fix queued, round-trip conversions, derive outcomes
  - Repository: ploy
  - Component: `internal/mods/api`
  - Scope: Either remove `StageStateQueued` from public types or map it consistently in both directions; consolidate conversions into explicit maps + helpers; derive API run outcome from real outcomes instead of mapping `Finished => Succeeded` unconditionally; define “unknown” behavior explicitly and test it (`internal/mods/api/status_conversion.go`) (`roadmap/refactor/mods-api.md`).
  - Snippets:
    - ```go
      var stageToStore = map[StageState]store.JobStatus{ ... }
      ```
  - Tests: `go test ./internal/mods/api -run TestStageStateMappingIsConsistent; go test ./internal/mods/api -run TestStatusConversionsRoundTrip; go test ./internal/mods/api -run TestRunStateFromStoreUsesOutcomes` — mappings are consistent and outcomes are derived

- [ ] Validate submit `spec` shape at the server boundary — require object-only when merge/inspect is needed
  - Repository: ploy
  - Component: `internal/server` + `internal/mods/api`
  - Scope: For endpoints accepting `RunSubmitRequest.Spec json.RawMessage`, require `json.Valid` and enforce object-only when the server merges/inspects it; cap request size (`roadmap/refactor/mods-api.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      if !isJSONObject(req.Spec) { return badRequest("spec must be an object") }
      ```
  - Tests: `go test ./internal/server/handlers -run TestSubmitSpecMustBeObject` — non-object specs return 400

## Worker (`internal/worker`) (typed IDs + hydration correctness + resource units)
- [ ] Type node IDs at the worker boundary — validate once, pass typed values
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Change collector options to take `domaintypes.NodeID` (or validate string before cast); remove unchecked casts (`internal/worker/lifecycle/collector.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      type Options struct{ NodeID domaintypes.NodeID }
      ```
  - Tests: `go test ./internal/worker/... -run TestCollectorRejectsEmptyNodeID` — invalid node id rejected

- [ ] Replace stringly-typed states with enums + Validate — prevent typos in lifecycle state
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Introduce `NodeState`/`ComponentState` types with constants and `Validate()`; update `internal/worker/lifecycle/types.go` and users (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      type NodeState string
      const NodeStateRunning NodeState = "running"
      ```
  - Tests: `go test ./internal/worker/... -run TestNodeStateValidate` — unknown states reject

- [ ] Replace `ComponentStatus.Details map[string]any` — use typed structs or `json.RawMessage` with validation
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Replace free-form details maps with per-component typed structs (or `json.RawMessage` if flexibility is required); validate known keys (`internal/worker/lifecycle/collector.go`, `internal/worker/lifecycle/types.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      type DockerDetails struct{ Containers int `json:"containers"` }
      ```
  - Tests: `go test ./internal/worker/... -run TestComponentDetailsTyped` — details decode/encode is type-safe

- [ ] Store network interfaces as a sorted slice — stabilize output ordering and reduce aliasing
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Replace `Interfaces map[string]NetworkInterface` with a `[]NetworkInterface` that includes a `Name` field; sort by name on write and deep-copy on read (`internal/worker/lifecycle/types.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].Name < ifaces[j].Name })
      ```
  - Tests: `go test ./internal/worker/... -run TestInterfacesSortedByName` — output order is stable

- [ ] Avoid allocating empty interface collections in snapshots — reduce churn and stabilize JSON output
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: When there are no interfaces, return `nil` (not an empty map/slice) from snapshot builders (e.g., `internal/worker/lifecycle/resources.go`); ensure encoding behavior is consistent (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      if len(ifaces) == 0 { ifaces = nil }
      ```
  - Tests: `go test ./internal/worker/... -run TestNoInterfacesProducesNil` — empty interface collections do not allocate

- [ ] Simplify `bumpToFrontLocked` — remove unnecessary sort + index scans
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Replace the current de-dupe + sort/index logic with a single-pass stable reordering; add unit test for ordering (`internal/worker/jobs/store.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      // build output by: wanted first (dedup), then rest (dedup)
      ```
  - Tests: `go test ./internal/worker/... -run TestBumpToFrontOrdering` — ordering is correct and stable

- [ ] Remove unused error return from `NewGitFetcher` — stop returning `(GitFetcher, error)` when it never errors
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Change signature to `NewGitFetcher(opts GitFetcherOptions) GitFetcher` and update call sites/tests (`internal/worker/hydration/git_fetcher.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      func NewGitFetcher(opts GitFetcherOptions) GitFetcher
      ```
  - Tests: `go test ./internal/worker/... -run TestNewGitFetcherSignature` — code compiles and tests pass

- [ ] Decide whether `Collect` can fail — return a real error or remove the unused error return
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Either return a non-nil error for hard failures (e.g., hostname) or remove the error return and standardize on warnings only (`internal/worker/lifecycle/collector.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      if err != nil { return NodeStatus{}, err }
      ```
  - Tests: `go test ./internal/worker/... -run TestCollectReturnsErrorOnHardFailure` — behavior is explicit and tested

- [ ] Use monotonic time for metrics deltas — stop wall-clock skew affecting rates
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Store `time.Time` values with monotonic component for delta calculations; only format UTC when encoding output (`internal/worker/lifecycle/metrics_cache.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      now := time.Now() // keep monotonic for Sub
      ```
  - Tests: `go test ./internal/worker/... -run TestMetricsDeltaUsesMonotonic` — deltas behave under simulated clock shifts

- [ ] Add nil receiver checks consistently — stop nil pointer panics in worker stores
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Add nil checks for `Get`/`List` matching `Start`/`Complete` behavior (`internal/worker/jobs/store.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      if s == nil { return nil, errors.New("store is nil") }
      ```
  - Tests: `go test ./internal/worker/... -run TestNilReceiverChecks` — nil receivers return errors, not panics

- [ ] Make hydration “already hydrated” check validate commit/baseRef — stop false positives
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Compare current HEAD commit and requested commit/base_ref before skipping hydration (`internal/worker/hydration/git_fetcher.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      head := gitRevParse(dest, "HEAD"); if head != want { return false }
      ```
  - Tests: `go test ./internal/worker/... -run TestHydrationChecksHeadCommit` — wrong commit triggers re-hydration

- [ ] Make hydration always land in an empty dir — stop stale files and partial writes
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: On copy failure, clean `dest` before falling back; hydrate into a temp dir then rename; avoid rsync into non-empty dir (`internal/worker/hydration/git_fetcher.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      tmp := dest + ".tmp"; defer os.RemoveAll(tmp); os.Rename(tmp, dest)
      ```
  - Tests: `go test ./internal/worker/... -run TestHydrationUsesTempDir` — failures do not leave partial/stale files

- [ ] Fix `filterInterfaces` name trimming — ensure ignore patterns match emitted names
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Assign trimmed name back into the struct before appending (`internal/worker/lifecycle/net_filters.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      stat.Name = strings.TrimSpace(stat.Name)
      ```
  - Tests: `go test ./internal/worker/... -run TestFilterInterfacesTrimsName` — stored names are trimmed

- [ ] Deep-copy cached lifecycle outputs — prevent shared map/slice mutation
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Ensure `LatestStatus` returns a deep copy of maps/slices (interfaces, details, etc.), or make cached structures immutable (`internal/worker/lifecycle/cache.go`) (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```go
      out := *s.latest; out.Interfaces = maps.Clone(s.latest.Interfaces)
      ```
  - Tests: `go test ./internal/worker/... -run TestLatestStatusIsDeepCopy` — caller mutation does not affect cache

- [ ] Delete accidental `.DS_Store` under worker — keep repo clean
  - Repository: ploy
  - Component: `internal/worker`
  - Scope: Delete `internal/worker/.DS_Store`; ensure `.DS_Store` is ignored (`.gitignore`) (see `roadmap/refactor/worker.md`).
  - Snippets:
    - ```bash
      rm -f internal/worker/.DS_Store
      ```
  - Tests: `git status --porcelain` — no `.DS_Store` tracked or untracked

- [ ] Add `.DS_Store` to `.gitignore` — prevent reintroduction
  - Repository: ploy
  - Component: repo root
  - Scope: Update `.gitignore` to ignore `.DS_Store` globally (`roadmap/refactor/worker.md`).
  - Snippets:
    - ```gitignore
      .DS_Store
      ```
  - Tests: `git status --porcelain` — creating `.DS_Store` does not show as untracked

## CLI shared (HTTP boundary + gzip streaming) (merged slice)
- [ ] Add a shared CLI HTTP helper — unify URL building, caps, strict JSON decode, and error shaping
  - Repository: ploy
  - Component: `internal/cli/*`
  - Scope: Create a shared helper package used by runs/mods/transfer; enforce no leading-slash JoinPath usage; cap error-body reads; strict JSON decode via `DisallowUnknownFields`; require non-infinite client timeouts (`roadmap/refactor/contracts.md`, `roadmap/refactor/scope.md`).
  - Snippets:
    - ```go
      endpoint, _ := url.JoinPath(base.String(), "v1", "runs", url.PathEscape(runID))
      dec := json.NewDecoder(io.LimitReader(resp.Body, maxBytes)); dec.DisallowUnknownFields()
      ```
  - Tests: `go test ./internal/cli/... -run TestHTTPHelperStrictDecode` — unknown fields and overlarge bodies are rejected

- [ ] Migrate Runs CLI to the shared HTTP helper — delete per-command boilerplate
  - Repository: ploy
  - Component: `internal/cli/runs`
  - Scope: Replace direct `http.NewRequest` + ad-hoc response decoding in runs commands/clients with the shared helper; delete duplicated error decoding and normalize URL building rules (`roadmap/refactor/scope.md`, `roadmap/refactor/cli-runs.md`).
  - Snippets:
    - ```go
      return httpx.DoJSON(ctx, client, req, &out)
      ```
  - Tests: `go test ./internal/cli/runs` — package compiles and unit tests pass

- [ ] Migrate Mods CLI to the shared HTTP helper — delete per-command boilerplate
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Replace direct `http.NewRequest` + ad-hoc response decoding in mods commands/clients with the shared helper; delete duplicated error decoding and normalize URL building rules (`roadmap/refactor/scope.md`, `roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      return httpx.DoJSON(ctx, client, req, &out)
      ```
  - Tests: `go test ./internal/cli/mods` — package compiles and unit tests pass

- [ ] Migrate Transfer CLI to the shared HTTP helper — delete per-command boilerplate
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Replace direct `http.NewRequest` + ad-hoc response decoding in transfer commands/clients with the shared helper; delete duplicated error decoding and normalize URL building rules (`roadmap/refactor/scope.md`, `roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      return httpx.DoJSON(ctx, client, req, &out)
      ```
  - Tests: `go test ./internal/cli/transfer` — package compiles and unit tests pass

- [ ] Add shared streaming gunzip helper for diff downloads — stop buffering entire gz payloads
  - Repository: ploy
  - Component: `internal/cli/*`
  - Scope: Implement one helper that streams `gzip.NewReader(resp.Body)` into a writer without reading the full body; reuse for runs/mods diff download (`roadmap/refactor/scope.md`, `roadmap/refactor/cli-runs.md`, `roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      zr, _ := gzip.NewReader(resp.Body); defer zr.Close()
      _, _ = io.Copy(dst, zr)
      ```
  - Tests: `go test ./internal/cli/runs -run TestDiffDownloadStreamsGunzip` — does not allocate full payload; output matches expected

## CLI stream (`internal/cli/stream`) (runtime correctness + de-duplication)
- [ ] Fix stream client cancel/request behavior — idle timeout correctness + cancel classification + no-cache header
  - Repository: ploy
  - Component: `internal/cli/stream`
  - Scope: Fix closure capture; stop using `defer cancelConn()` inside reconnect loops; stop/drain timers per iteration; classify idle-timeout vs parent ctx cancellation correctly; ensure SSE requests set `Cache-Control: no-cache` (`internal/cli/stream/client.go`, `internal/cli/stream/sse_client.go`) (`roadmap/refactor/cli-stream.md`).
  - Snippets:
    - ```go
      cancel := cancelConn
      timer := time.AfterFunc(idle, func() { cancel() })
      ```
  - Tests: `go test ./internal/cli/stream -run TestIdleTimeoutDoesNotCancelNewConn; go test ./internal/cli/stream -run TestCancelIsNotIdleTimeout; go test ./internal/cli/stream -run TestRequestHasNoCacheHeader` — idle timeout is per-conn; user cancel is ctx cancel; header is set

- [ ] Decide `retry:` hint policy — either implement it or delete claims/fields
  - Repository: ploy
  - Component: `internal/cli/stream`
  - Scope: Either switch to an SSE reader that exposes `retry` hints and respect them, or remove `Retry`-hint claims from docs/comments and delete any dead fields (`roadmap/refactor/cli-stream.md`).
  - Snippets:
    - ```go
      // Option A: remove Retry field + comments if not supported.
      ```
  - Tests: `go test ./internal/cli/stream -run TestRetryHintPolicy` — behavior is explicit and tested

- [ ] Make `MaxEventSize` configurable — stop hard-coding 1 MiB in clients
  - Repository: ploy
  - Component: `internal/cli/stream`
  - Scope: Thread `MaxEventSize` through config/flags/env and document expectations; keep a safe default (`internal/cli/stream/client.go`) (`roadmap/refactor/cli-stream.md`).
  - Snippets:
    - ```go
      type Options struct{ MaxEventSize int64 }
      ```
  - Tests: `go test ./internal/cli/stream -run TestMaxEventSizeConfigurable` — custom size takes effect

- [ ] Remove duplicate client implementation — keep one streaming client + one backoff policy
  - Repository: ploy
  - Component: `internal/cli/stream`
  - Scope: Delete the unused/duplicate client (`Client` vs `SSEClient`) and its tests; unify on one backoff implementation (`internal/workflow/backoff`) (`roadmap/refactor/cli-stream.md`).
  - Snippets:
    - ```go
      // Delete: internal/cli/stream/sse_client.go
      ```
  - Tests: `go test ./internal/cli/stream` — package builds and tests pass after deletion

## CLI logs (`internal/cli/logs`) (canonical payloads + concurrency correctness)
- [ ] Switch logs to canonical `internal/stream.LogRecord` — stop duplicate payload structs
  - Repository: ploy
  - Component: `internal/cli/logs` + `internal/stream`
  - Scope: Remove `internal/cli/logs.LogRecord`; decode into `internal/stream.LogRecord` (typed `mod_type`/`step_index` per contracts) (`roadmap/refactor/cli-logs.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      var rec logstream.LogRecord
      ```
  - Tests: `go test ./internal/cli/logs -run TestLogRecordDecodeUsesCanonicalType` — decode compiles and uses canonical struct

- [ ] Make Printer concurrency story explicit — add locking or remove “thread-safe” claim
  - Repository: ploy
  - Component: `internal/cli/logs`
  - Scope: Either add a mutex around `retention` + output writes, or remove the thread-safe claim and enforce single-goroutine usage (`internal/cli/logs/printer.go`) (`roadmap/refactor/cli-logs.md`).
  - Snippets:
    - ```go
      p.mu.Lock(); defer p.mu.Unlock()
      ```
  - Tests: `go test ./internal/cli/logs -race` — no data races on concurrent calls

- [ ] Treat “unset step_index” explicitly — stop printing logic relying on `> 0` sentinel behavior
  - Repository: ploy
  - Component: `internal/cli/logs`
  - Scope: Once `step_index` is `types.StepIndex`, represent “unset” explicitly (e.g., pointer field) and adjust print logic; remove `> 0` heuristics (`internal/cli/logs/printer.go`) (`roadmap/refactor/cli-logs.md`).
  - Snippets:
    - ```go
      StepIndex *types.StepIndex `json:"step_index,omitempty"`
      ```
  - Tests: `go test ./internal/cli/logs -run TestPrinterStepIndexUnset` — unset does not print; set prints consistently

- [ ] Centralize SSE event decoding used by `mods`, `runs`, and `logs` — stop per-command drift
  - Repository: ploy
  - Component: `internal/cli/*`
  - Scope: Implement one SSE event decode path (event type allow-list + canonical payload structs) and reuse in `internal/cli/mods/events.go`, `internal/cli/runs/follow.go`, and `internal/cli/logs/*` (`roadmap/refactor/scope.md`, `roadmap/refactor/cli-logs.md`).
  - Snippets:
    - ```go
      func DecodeEvent(evt sse.Event) (types.SSEEventType, []byte, error)
      ```
  - Tests: `go test ./internal/cli/... -run TestSSEDecodeSharedContract` — all stream consumers decode the same contract

## CLI runs (`internal/cli/runs`) (typed IDs + strict decode + cancel semantics)
- [ ] Type `repo_id` as `types.ModRepoID` — stop raw strings in CLI inputs
  - Repository: ploy
  - Component: `internal/cli/runs`
  - Scope: Change `RepoDiffsCommand.RepoID` from `string` to `types.ModRepoID` and validate before URL building (`internal/cli/runs/diffs.go`) (`roadmap/refactor/cli-runs.md`).
  - Snippets:
    - ```go
      if c.RepoID.IsZero() { return errors.New("repo_id required") }
      ```
  - Tests: `go test ./internal/cli/runs -run TestRepoDiffsRequiresRepoID` — empty repo_id fails before HTTP

- [ ] Decode diff timestamps as `time.Time` and sort locally — stop relying on server ordering implicitly
  - Repository: ploy
  - Component: `internal/cli/runs`
  - Scope: Change diff list structs to use `time.Time` for `created_at` and sort when selecting “newest” (`internal/cli/runs/diffs.go`) (`roadmap/refactor/cli-runs.md`).
  - Snippets:
    - ```go
      sort.Slice(diffs, func(i, j int) bool { return diffs[i].CreatedAt.After(diffs[j].CreatedAt) })
      ```
  - Tests: `go test ./internal/cli/runs -run TestDiffNewestSelectionSortsByTime` — newest selection is correct even if API order changes

- [ ] Remove StopCommand and keep cancel semantics only — stop having two commands for the same endpoint
  - Repository: ploy
  - Component: `internal/cli/runs` + cmd wiring
  - Scope: Remove `StopCommand`; keep `CancelCommand` only; treat `202 Accepted` as success; remove aliases (`roadmap/refactor/cli-runs.md`).
  - Snippets:
    - ```go
      // Delete: internal/cli/runs/stop.go
      ```
  - Tests: `go test ./internal/cli/runs` — builds after deleting stop command; cancel tests pass

## CLI mods (`internal/cli/mods`) (typed IDs + strict decode + remove heuristics)
- [ ] Type all Mods CLI identifiers — eliminate raw string IDs in commands and responses
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Replace raw `string` IDs with domain types (`types.RunID`, `types.ModRepoID`, `types.ModID`, `types.ModRef`, `types.UUID`/new `DiffID`); validate before URL building (`roadmap/refactor/cli-mods.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      type DownloadDiffCommand struct { RepoID types.ModRepoID; DiffID types.UUID }
      ```
  - Tests: `go test ./internal/cli/mods -run TestCommandsValidateTypedIDs` — invalid IDs rejected before HTTP

- [ ] Type Mods CLI VCS inputs and validate lists — stop “non-empty string” validation
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Use `types.RepoURL` and `types.GitRef` directly in request structs; validate `RepoURLs` items individually; stop deferring validation to later codepaths (`internal/cli/mods/mod_repos.go`, `internal/cli/mods/mod_run.go`, `internal/cli/mods/batch.go`) (`roadmap/refactor/cli-mods.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      type AddModRepoRequest struct { RepoURL types.RepoURL `json:"repo_url"` }
      ```
  - Tests: `go test ./internal/cli/mods -run TestRepoURLsValidateIndividually` — invalid entries reject with index/field path

- [ ] Decode Mods CLI timestamps as `time.Time` — enforce RFC3339 and enable correct ordering
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Change `CreatedAt string` fields in CLI response structs to `time.Time` and update formatting/printing (`internal/cli/mods/mod_management.go`, `internal/cli/mods/mod_repos.go`) (`roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      CreatedAt time.Time `json:"created_at"`
      ```
  - Tests: `go test ./internal/cli/mods -run TestModsCLITimestampsDecodeStrict` — invalid timestamps reject

- [ ] Validate and escape all path params consistently — stop mixing raw and escaped segments
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Ensure all dynamic path segments are validated URL-safe by type, then `url.PathEscape`’d consistently; remove ad-hoc mixtures (`roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      endpoint := base.JoinPath("v1", "mods", url.PathEscape(modRef.String()))
      ```
  - Tests: `go test ./internal/cli/mods -run TestPathParamsEscaped` — unsafe ids are rejected or escaped consistently

- [ ] Remove “UUID-like means ID” heuristic — rely on explicit server resolution
  - Repository: ploy
  - Component: `internal/cli/mods`
  - Scope: Delete the heuristic in `ResolveModByNameCommand`; always treat user input as a `types.ModRef` and let the server resolve (`internal/cli/mods/mod_management.go`) (`roadmap/refactor/cli-mods.md`).
  - Snippets:
    - ```go
      // No local heuristics; always call resolve endpoint with ModRef.
      ```
  - Tests: `go test ./internal/cli/mods -run TestResolveDoesNotAssumeUUID` — UUID-looking names still resolve by name

- [ ] Update artifacts/status code for typed stage keys (`types.JobID`) — stop treating keys as arbitrary strings
  - Repository: ploy
  - Component: `internal/cli/mods` + `internal/mods/api`
  - Scope: After `modsapi.RunSummary.Stages` is `map[types.JobID]...`, update CLI code to use typed job ids (`internal/cli/mods/artifacts.go`) (`roadmap/refactor/cli-mods.md`, `roadmap/refactor/mods-api.md`).
  - Snippets:
    - ```go
      for jobID, stage := range summary.Stages { _ = jobID /* typed */ }
      ```
  - Tests: `go test ./internal/cli/mods -run TestArtifactsUsesTypedStageKeys` — compiles and handles typed keys

## CLI transfer (`internal/cli/transfer`) (typed requests + safe URL handling + timeouts)
- [ ] Replace stringly-typed kind/stage/slot/digest — validate before requests
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Switch request structs to `types.TransferKind`/`types.TransferStage`/`types.SlotID`/`types.Digest` and validate before sending (`internal/cli/transfer/client.go`) (`roadmap/refactor/cli-trasnfer.md`, `roadmap/refactor/contracts.md`).
  - Snippets:
    - ```go
      if err := req.Kind.Validate(); err != nil { return err }
      ```
  - Tests: `go test ./internal/cli/transfer -run TestTransferValidateBeforeHTTP` — invalid kind/stage/digest rejected locally

- [ ] Remove `requestSlot(any)` runtime type switching — keep compile-time typed helpers
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Replace `requestSlot(payload any)` with two typed methods (or route both through a typed `doReq`); remove `any` + switch (`internal/cli/transfer/client.go`) (`roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      func (c *Client) requestUpload(ctx context.Context, req UploadSlotRequest) (Slot, error)
      ```
  - Tests: `go test ./internal/cli/transfer -run TestRequestSlotTypedHelpers` — upload/download slot requests are compile-time typed

- [ ] Consolidate commit/abort request building — ensure consistent validation and error shaping
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Centralize slot action endpoint construction + request execution used by `Commit` and `Abort` (`internal/cli/transfer/client.go`) (`roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      func (c *Client) slotAction(ctx context.Context, slotID SlotID, action string, payload any) error
      ```
  - Tests: `go test ./internal/cli/transfer -run TestCommitAbortShareValidation` — both actions validate identically

- [ ] Fix URL path handling for slot actions — no `path.Join` normalization for untrusted segments
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Validate `slot_id` is URL-safe; use `url.PathEscape` when interpolating; avoid normalizing `..` via `path.Join` (`internal/cli/transfer/client.go`) (`roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      endpoint, _ := url.JoinPath(base.String(), "v1", "slots", url.PathEscape(slotID.String()), "commit")
      ```
  - Tests: `go test ./internal/cli/transfer -run TestSlotIDCannotContainSlash` — slot IDs with `/` are rejected

- [ ] Require HTTP timeouts for transfer client — stop hanging uploads/downloads
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Refuse `http.DefaultClient` without a timeout; enforce a non-zero `http.Client.Timeout` in constructors (`internal/cli/transfer/client.go`) (`roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      if c.HTTPClient == nil || c.HTTPClient.Timeout == 0 { return errors.New("http timeout required") }
      ```
  - Tests: `go test ./internal/cli/transfer -run TestTransferClientRequiresTimeout` — zero-timeout clients reject

- [ ] Cap and parse error bodies consistently — stop reading entire bodies on non-2xx responses
  - Repository: ploy
  - Component: `internal/cli/transfer`
  - Scope: Use the shared CLI HTTP error shaping rules (cap reads; parse `{ "error": "..." }` when present; fallback to trimmed text/status) (`internal/cli/transfer/client.go`) (`roadmap/refactor/contracts.md`, `roadmap/refactor/cli-trasnfer.md`).
  - Snippets:
    - ```go
      return httpx.DecodeError(resp, "transfer")
      ```
  - Tests: `go test ./internal/cli/transfer -run TestErrorBodiesCapped` — large error bodies do not allocate unbounded memory

## Validation (repo-wide)
- [ ] Run repo-wide tests + TDD discipline validation after each slice — keep coverage and guardrails intact
  - Repository: ploy
  - Component: repo-wide
  - Scope: After each “merged slice” (SSE/log payload contract; CLI HTTP helper; migrations), run unit tests, coverage, and static analysis and fix only failures caused by the slice (`AGENTS.md` policy).
  - Snippets:
    - ```bash
      make test
      ./scripts/validate-tdd-discipline.sh
      ```
  - Tests: `make test` — all tests pass; coverage thresholds remain met
