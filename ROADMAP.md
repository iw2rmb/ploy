# Type System Hardening for Internal Packages

Scope: Tighten the Go type system in `internal` packages for clarity and maintainability without changing external JSON/wire contracts. Focus on lifecycle snapshots, nodeagent options/specs, ID/VCS validation at server boundaries, enum/status consistency, and removal of untyped extension points.

Documentation: `GOLANG.md`, `ROADMAP_NEXT.md`, `docs/api/OpenAPI.yaml`, `docs/envs/README.md`

Legend: [ ] todo, [x] done.

## Lifecycle Snapshots
- [x] Introduce typed lifecycle snapshot structs — Harden resource/status schema and reduce `map[string]any` casts
  - Component: `internal/worker/lifecycle`, `internal/server/status`, `internal/nodeagent`
  - Scope: Add `NodeStatus` / `NodeCapacity` structs in `internal/worker/lifecycle`; add helpers to convert to/from `map[string]any`; update `Collector.Collect`, `Cache`, `status.Provider`, and `HeartbeatManager.sendHeartbeat` to use typed accessors instead of direct `map[string]any` indexing
  - Test: `go test ./internal/worker/lifecycle ./internal/server/status ./internal/nodeagent` — Heartbeat and status snapshot tests continue to pass with unchanged JSON payloads

## Nodeagent Run Options
- [ ] Introduce typed RunOptions for nodeagent execution — Clarify which spec/options keys are understood by the agent
  - Component: `internal/nodeagent`
  - Scope: Define small option structs (e.g., build gate config, healing config, MR wiring) in a new file; update `parseSpec` and `buildManifestFromRequest` to populate typed options while preserving raw JSON where needed
  - Test: `go test ./internal/nodeagent` — Healing, MR creation, and manifest builder tests continue to pass; JSON contracts remain stable

## ID and VCS Validation
- [ ] Use domain ID/VCS types at server boundaries — Centralize validation for repo URLs, refs, and identifiers
  - Component: `internal/server/handlers`, `internal/server/auth`, `internal/cli/config`
  - Scope: Replace plain `string` fields with `domaintypes.RepoURL`, `GitRef`, `CommitSHA`, and `ClusterID` in handler request/response structs and token claims where JSON stays string-based; add minimal conversion helpers for CLI config if needed
  - Test: `go test ./internal/server/... ./internal/cli/...` — Mods ticket submission, claim, and auth tests pass; OpenAPI docs and CLI behavior remain unchanged

## Status / Enum Consistency
- [ ] Align enum/status types across store, workflow contracts, and mods API — Reduce string casts and duplicated status definitions
  - Component: `internal/store`, `internal/workflow/contracts`, `internal/mods/api`, `internal/server/handlers`
  - Scope: Introduce shared domain enums for run/stage/buildgate job status or adjust `sqlc.yaml` to reuse existing contract enums; update handlers to rely on shared types instead of ad hoc string conversions
  - Test: `go test ./internal/store ./internal/workflow/contracts ./internal/mods/api ./internal/server/handlers` — All status transition tests pass and JSON status fields remain identical

## Git Fetcher Publisher Hook
- [ ] Narrow or remove GitFetcherOptions.Publisher — Eliminate untyped extension point
  - Component: `internal/worker/hydration`, `internal/nodeagent`
  - Scope: Either remove the `Publisher` field from `GitFetcherOptions` (if unused) or replace `interface{}` with a small `SnapshotPublisher` interface; update `NewGitFetcher` callers and tests
  - Test: `go test ./internal/worker/hydration ./internal/nodeagent` — Git fetcher behavior and buildgate executor tests continue to pass
