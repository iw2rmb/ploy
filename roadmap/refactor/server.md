# Server Refactor Notes (`internal/server`)

This file tracks remaining server refactor work that is not yet reflected in `docs/` at HEAD.

## Remaining Work

- Spec merge strictness:
  - Reject invalid JSON and non-object JSON when merging spec blobs (no silent `{}` fallback).
  - Likely touch points: `internal/server/handlers/spec_utils.go`, `internal/server/handlers/nodes_claim.go`.
- Heartbeat contract:
  - Switch to integer + unit-explicit fields and enforce strict decode + invariants.
- Replace mods `repo_url` filter N+1 with a store query:
  - Update `internal/server/handlers/mods.go` to call a JOIN/EXISTS query rather than listing repos per mod.
- Harden token authorizer side effects:
  - Make revocation idempotent via `errors.Is(err, pgx.ErrNoRows)` and avoid unbounded goroutines for “last used” updates.
- Fix config watcher debounce/reload lifetime:
  - Ensure timers don’t fire after shutdown; avoid `context.Background()` reloads.
