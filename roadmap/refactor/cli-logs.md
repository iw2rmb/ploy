# CLI Logs Refactor Notes (`internal/cli/logs`)

- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs/newtypes, StepIndex, SSE contract).
- Merged work item: SSE + log payload contract is implemented as one slice (see `roadmap/refactor/scope.md`); this file only tracks `internal/cli/logs` specifics once the canonical payload/types exist.

## Type Hardening

- Adopt the canonical SSE payload structs and domain types.
  - Merged slice: replace `internal/cli/logs.LogRecord` with the canonical `internal/stream.LogRecord` (and typed `step_index`/`mod_type`/retention fields) per `roadmap/refactor/contracts.md` and `roadmap/refactor/scope.md`.

## Streamlining / Simplification

- Consolidate shared SSE decoding paths (merged slice).
  - Centralize SSE event decoding once (so `mods`, `runs`, and `logs` don’t drift).
- Avoid claiming thread-safety without synchronization.
  - `Printer` stores `retention *RetentionHint` and writes to `out` without locking (`internal/cli/logs/printer.go:62`), but the comment says “Thread-safe”.
  - Solution: either add a mutex to `Printer` (guard `PrintLog`, `RecordRetention`, `PrintRetentionSummary`) or remove the thread-safe claim and enforce single-goroutine usage.

## Likely Bugs / Risks

- Data race potential if used concurrently.
  - `RecordRetention` mutates `p.retention` while `PrintRetentionSummary` reads it; concurrent calls can race (`internal/cli/logs/printer.go:158`, `internal/cli/logs/printer.go:165`).
  - Solution: add locking (or make retention immutable and deliver it only after streaming completes).
- `step_index` omission policy is ad-hoc.
  - The printer only renders `step_index` when `> 0` (`internal/cli/logs/printer.go:132`), which assumes “0 means absent”.
  - Solution: once `step_index` is `types.StepIndex`, treat “unset” explicitly (e.g., `nil` pointer) rather than relying on numeric sentinel behavior.
