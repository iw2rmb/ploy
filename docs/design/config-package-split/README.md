# Config package split

## Why
- `internal/api/config/config.go` ballooned to ~442 LOC and houses structs, load logic, defaults, runtime normalization, and validation in one place.
- The size makes reviews noisy, discourages targeted tweaks, and hides related helpers behind unrelated changes.
- Splitting restores topical cohesion so future control-plane or runtime tweaks stay localized.

## What to do
1. Keep `config.go` focused on the exported `Config` struct definition and top-level wiring; move the subordinate structs into `types.go` grouped by concern (HTTP, PKI, runtime, etc.).
2. Move the constants, `defaultConfig`, and `applyDefaults` logic into `defaults.go`; ensure raw plugin bookkeeping stays with defaults because it runs post-load.
3. Create `loader.go` housing `Load`, `loadFromReader`, and `ResolveRelative`; add focused tests around `ResolveRelative` if gaps appear.
4. Move runtime-specific helpers (`RuntimeConfig`, `RuntimePluginConfig`, `normalizeRuntimeConfig`, `ptrTo`) into `runtime.go`, keeping runtime behavior isolated.
5. Place `validate` into `validate.go`, limiting imports to the validation needs.
6. Update `internal/api/config/config.go` imports accordingly and ensure Go build tags/lint stay satisfied.
7. Keep package public API identical; no behavior change, just file organization.

## Where to change
- `docs/design/config-package-split/README.md` (this document).
- `docs/design/QUEUE.md` (entry already marked as claimed).
- `internal/api/config/config.go` (prune sections now moved and host minimal glue).
- New files: `internal/api/config/types.go`, `defaults.go`, `loader.go`, `runtime.go`, `validate.go`.

## COSMIC evaluation
| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Config load refactor | 0 | 0 | 0 | 0 | 0 |
| TOTAL | 0 | 0 | 0 | 0 | 0 |

Assumptions: purely structural refactor, no net-new data movements. No external interfaces change.

## How to test
- `go test ./internal/api/config`.
- If time permits, `make test` to ensure repo-wide guardrails still pass.
