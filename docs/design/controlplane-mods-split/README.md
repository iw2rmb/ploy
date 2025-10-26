# Control Plane Mods Split

Design for decomposing `internal/api/httpserver/controlplane_mods.go`, currently the largest Go source file in the repo.

## Status

| Date (UTC) | State | Notes |
| --- | --- | --- |
| 2025-10-26 | Draft | Targeting approval to refactor the MODS handlers into cohesive files. |

## References

- `internal/api/httpserver/controlplane_mods.go`
- `internal/api/httpserver/controlplane_jobs_routes.go`
- `docs/design/controlplane-registry-refactor/README.md`

## Context

- `controlplane_mods.go` holds routing, mutation handlers, streaming SSE loops, log helpers, and DTO conversion code across 400+ LOC. This makes reviews slow and discourages incremental changes.
- The MODS HTTP surface mirrors the scheduler endpoints (`controlplane_jobs_*` files) but lacks their structure, so new handlers copy/paste patterns rather than reusing helpers.
- Existing tests (`internal/api/httpserver/controlplane_mods_test.go`) couple to package-level helpers; splitting without a plan risks duplicate logic and regressions around SSE/log streaming.

## Objectives

1. Route handlers live in a small `*_routes.go` companion similar to the jobs API to keep dispatch logic easy to scan.
2. Mutations (submit, cancel, resume) and status lookups move into their own file so business validation can evolve independently.
3. Streaming endpoints (logs snapshot/stream, SSE events) shift into targeted files to isolate long handlers from simple HTTP helpers.
4. Shared DTO/clone helpers move into `controlplane_mods_helpers.go` to centralize conversions for both HTTP and tests.

## Proposed Layout

| File | Responsibility |
| --- | --- |
| `controlplane_mods_routes.go` | `handleModsSubpath`, `handleModsTickets`, `handleModsTicketSubpath`, `handleModsLogs` dispatcher mirroring job routes. |
| `controlplane_mods_mutations.go` | `handleModsSubmit`, `handleModsTicketStatus`, `handleModsCancel`, `handleModsResume`, request validation helpers. |
| `controlplane_mods_logs.go` | `handleModsLogsSnapshot`, `handleModsLogsStream`, log stream plumbing, snapshot DTO assembly. |
| `controlplane_mods_events.go` | `handleModsEvents`, SSE pump, ticker keepalives, mods watch wiring. |
| `controlplane_mods_helpers.go` | `mapModsError`, DTO transformers, `cloneStringMap`, `cloneStringSlice`, `modsStageEvent`. |

Each file should stay under ~200 LOC, carry one-liner function comments, and leave existing exported surface untouched.

## Execution Plan

1. Create the new files listed above, move functions wholesale, and add doc comments without changing logic.
2. Keep `controlplane_mods.go` only if needed for shared types; otherwise delete it after moves.
3. Update `controlplane_mods_test.go` imports if needed (expected to compile without edits because files share package).
4. Run `gofmt` on the new files to keep style consistent.

## Testing & Coverage

- `go test ./internal/api/httpserver -run Mods`
- `go test ./internal/api/httpserver`

Both commands keep the RED→GREEN loop local while ensuring the rest of the HTTP server package still compiles and passes after the split.

## Risks & Mitigations

- **Large diff noise** — keep functions identical aside from import blocks and comments to make review trivial.
- **SSE/log regressions** — rely on `controlplane_mods_test.go` coverage and avoid touching helper logic beyond moves.
