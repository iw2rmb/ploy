# Snapshot Toolkit

## Purpose
Describe how the snapshot toolkit transforms database fixtures during the SHIFT reboot, keeping captures deterministic while the Grid/IPFS integration is stubbed locally.

## Current Status
- `ploy snapshot plan` and `ploy snapshot capture` are available locally and operate against fixtures defined in `configs/snapshots/*.toml`.
- Strip, mask, and synthetic rules execute via the in-memory rule engine; metadata flows to the JetStream/IPFS stubs until real services are wired in.
- Container-backed replays are deferred to the JetStream integration slice; captures currently rely on deterministic JSON fixtures.

## Usage / Commands
- `ploy snapshot plan --snapshot <snapshot-name>` — Summarises strip/mask/synthetic rules, tables touched, and highlights before a capture runs.
- `ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>` — Applies the rule engine, emits a deterministic fingerprint, publishes metadata to the JetStream stub, and returns the fake IPFS CID.

## Development Notes
- Specs use TOML:
  ```toml
  name = "dev-db"
  description = "Development Postgres snapshot"

  [source]
  engine = "postgres"
  dsn = "postgres://localhost/dev"
  fixture = "dev-db.json"

  [[strip]]
  table = "users"
  columns = ["ssn"]

  [[mask]]
  table = "users"
  column = "email"
  strategy = "hash"

  [[synthetic]]
  table = "users"
  column = "token"
  strategy = "uuid"
  ```
- Fixtures are JSON maps of table name → rows; the engine coerces values to strings before applying rules.
- Supported strategies:
  - Mask: `hash` (SHA-256 truncated) and `redact` (literal `REDACTED`).
  - Synthetic: `uuid` (deterministic `uuid-<table>-<row>` tokens) and `static` (`STATIC`).
- `internal/workflow/snapshots` exposes `LoadDirectory`, `Plan`, and `Capture` helpers for other packages; tests keep coverage ≥90% to honour the critical path status.

## Related Docs
- `docs/design/shift/README.md` — Overall SHIFT architecture.
- `README.md` — High-level CLI overview (snapshot commands included).
- `cmd/ploy/README.md` — CLI usage details and environment placeholders.
