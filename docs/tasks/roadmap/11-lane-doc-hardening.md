# Lane Documentation Hardening

- [x] Done (2025-09-26)

## Why / What For

Lock the lane specification contract ahead of the Grid workflow wiring by
enforcing mandatory metadata and keeping the CLI documentation aligned with real
output.

## Required Changes

- Reject lane specs that omit a `description` so contributors cannot add
  incomplete profiles.
- Update `docs/LANES.md` with the required/optional field matrix and a fresh
  `ploy lanes describe` sample output.
- Sync the CLI example with the current cache-key format to prevent drift as the
  schema evolves.

## Definition of Done

- `internal/workflow/lanes` fails fast on lane descriptors missing a
  description.
- `docs/LANES.md` documents field requirements and shows accurate CLI output for
  `go-native`.
- Roadmap entry recorded for the workstation slice so future tasks build on the
  hardened schema.

## Tests

- RED → GREEN: `go test ./internal/workflow/lanes` (new validation test).
- Verification: `go test -cover ./...`.
