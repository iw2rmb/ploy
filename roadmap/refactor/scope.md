# Refactor Scope (`roadmap/refactor`)

## Executive Summary

- Primary goal: make API + internal contracts strict and type-safe by switching to `internal/domain/types` end-to-end, eliminating drift between server/store/stream/CLI.
- Correctness priorities:
  - Fix stream hub “send on closed channel” risk (`roadmap/refactor/stream.md`).
  - Fix `step_index` semantics and remove lossy `float64 -> int` casts (`roadmap/refactor/contracts.md`, `roadmap/refactor/server.md`, `roadmap/refactor/mods-api.md`, `roadmap/refactor/workflow.md`).
  - Make heartbeat units integer + explicit and align nodeagent/server/store (`roadmap/refactor/server.md`).
  - Make store migrations tracked and deterministic (Option A only) (`roadmap/refactor/store.md`).
- Non-goals (per project policy): no backward-compat layers, no migration planning outside Option A, no “keep old algorithm” fallbacks.

## Items Worth Merging / Unifying

- **SSE + log payload contract** (merge the work item across docs).
  - Duplicate concerns are spread across `contracts.md`, `stream.md`, `cli-stream.md`, `cli-logs.md`, `server.md`, and `mods-api.md`.
  - Unify into one implementation slice:
    - Define canonical payload structs (`internal/stream.LogRecord`), typed cursor (`types.EventID`), typed `step_index` (`types.StepIndex`), typed `mod_type` (`types.ModType`), and a closed set of event types.
    - Update server publish path + CLI decode paths together so drift is impossible.
  - Benefits: removes repeated per-package “fix step_index / mod_type / cursor” tasks.
  - Risk: touches multiple packages; requires coordinated tests across server + cli + stream.
- **CLI HTTP client behavior** (merge the work item across CLI packages).
  - Repeated issues appear in `cli-mods.md`, `cli-runs.md`, and `cli-trasnfer.md`: URL building with leading `/`, inconsistent error-body caps, and non-strict JSON decoding.
  - Unify into one shared helper + one rule set (caps + `DisallowUnknownFields`) used by all CLI commands.
  - Benefits: prevents drift between command implementations and reduces duplicated tests.
  - Risk: wide CLI blast radius; must keep behavior strict (no aliases, no hidden compatibility).
- **Repo-scoped identifiers** (merge the work item across runs/mods/transfer).
  - `repo_id` appears as raw `string` in multiple CLI and API surfaces (`cli-runs.md`, `cli-mods.md`, `mods-api.md`).
  - Unify by using `types.ModRepoID` everywhere internally and in CLI command structs.
- **Gzip diff download** (merge the work item across CLI commands).
  - Both `cli-runs.md` and `cli-mods.md` call out “buffer whole gz, then gunzip”.
  - Unify by implementing streaming decompression once and reusing it.

## Implementation Order (Docs → Code)

1. `roadmap/refactor/scope.md` — global slice plan and merge points.
2. `roadmap/refactor/contracts.md` — lock the end-state types and invariants (IDs, StepIndex, SSE cursor/types, heartbeat units).
3. `roadmap/refactor/store.md` — migrations (Option A), `search_path`, sqlc overrides to domain newtypes.
4. `roadmap/refactor/server.md` — strict JSON boundaries + heartbeat unit contract, remove/validate redundant identity fields, stop lossy casts.
5. `roadmap/refactor/stream.md` — fix hub safety + type hardening for events/log payloads and cursor.
6. `roadmap/refactor/workflow.md` — remove `mod_index` from specs; fix parsing truncations; enforce StepIndex invariants in graph/healing.
7. `roadmap/refactor/mods-api.md` — domain-typed request/response structs; stage map keys; StepIndex and ModType typing.
8. `roadmap/refactor/worker.md` — hydrate into empty dir; align resources/units and typed IDs.
9. `roadmap/refactor/cli-stream.md` — fix reconnect/idle correctness; typed cursor and event types.
10. `roadmap/refactor/cli-logs.md` — single canonical log payload type; typed StepIndex/ModType; retention typing.
11. `roadmap/refactor/cli-runs.md` — enforce strict cancel semantics; type `repo_id`; normalize URL building and decoding.
12. `roadmap/refactor/cli-mods.md` — type IDs/refs; remove incorrect mod-id heuristic; normalize URL building/decoding.
13. `roadmap/refactor/cli-trasnfer.md` — typed slot/kind/stage/digest; safe URL path handling; strict decode.

