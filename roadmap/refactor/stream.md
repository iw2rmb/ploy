# Stream Refactor Notes (`internal/stream`)

- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs, JSON boundaries, StepIndex).
- Merged work item: SSE + log payload contract (cursor/event types/payload structs) is implemented as one slice (see `roadmap/refactor/scope.md`); this file focuses on hub safety and stream-specific algorithms.

## Type Hardening

- Apply typed stream IDs/cursor/event types/payload structs (merged slice).
  - Implement stream/event typing per `roadmap/refactor/contracts.md` and apply it consistently in hub + HTTP + server integration.

## Streamlining / Simplification

- Remove duplication between `Serve` and `ServeFiltered`.
  - The functions are nearly identical (`internal/stream/http.go:14`, `internal/stream/http.go:63`).
  - Solution: keep one implementation with an optional filter function.
- Make history retention O(1) without full-slice copies.
  - Today `publish` truncates history by allocating and copying a full `[]Event` when it exceeds `HistorySize` (`internal/stream/hub.go:312`).
  - Solution: use a ring buffer (or keep a moving start index) so retention does not allocate on steady-state streams.
- Avoid linear scans for history selection.
  - Today `historyAfterLocked` scans all retained events each subscribe (`internal/stream/hub.go:398`).
  - Solution: since `Event.ID` is monotonic and history is ordered, compute the start offset via binary search.
- Avoid `string(evt.Data)` conversions in SSE framing hot path.
  - `writeEventFrame` converts arbitrary bytes to string and splits (`internal/stream/http.go:115`); fuzzing covers non-UTF8 input (`internal/stream/http_fuzz_test.go:10`).
  - Solution: split on `\n` at the byte level and write bytes directly to avoid extra allocations.

## Likely Bugs / Risks

- **Possible panic: send on closed channel**.
  - `subscriber.send` closes the channel on backpressure (`internal/stream/hub.go:424`), and `drop` / `finish` also closes subscriber channels (`internal/stream/hub.go:365`, `internal/stream/hub.go:380`).
  - `stream.publish` snapshots subscribers, unlocks, then sends (`internal/stream/hub.go:304`); another goroutine can close a subscriber channel after the snapshot but before the send, causing a panic on `s.ch <- evt`.
  - Solution: ensure sends never race with closes (e.g., do not close in `send`; only close after removal under the stream lock and make `send` safe against concurrent close).
- Silent no-op on publish with empty stream ID.
  - `publish` returns `nil` on blank stream ID (`internal/stream/hub.go:150`) which can hide upstream bugs (callers think log/event was published).
  - Solution: return an error for blank stream IDs at publish boundaries (or make stream ID a validated newtype so blank is impossible).
