# Snapshot Toolkit

## Purpose
Describe how the snapshot toolkit transforms database fixtures during the SHIFT reboot, keeping captures deterministic while supporting real IPFS publishing when a gateway is available.

## Current Status
- `ploy snapshot plan` and `ploy snapshot capture` are available locally and operate against fixtures defined in `configs/snapshots/*.toml`.
- Strip, mask, and synthetic rules execute via the in-memory rule engine; metadata publishes to JetStream when Grid discovery returns routes and falls back to the in-memory stub otherwise.
- Artifact payloads upload to the IPFS gateway reported by discovery; when no gateway is returned, the deterministic in-memory publisher yields repeatable fake CIDs for offline development.
- Container-backed replays are deferred to the JetStream integration slice; captures currently rely on deterministic JSON fixtures.
- Nomad snapshot tooling is retired; IPFS/JetStream publishing described here replaces SeaweedFS pipelines.

## Usage / Commands
- `ploy snapshot plan --snapshot <snapshot-name>` — Summarises strip/mask/synthetic rules, tables touched, and highlights before a capture runs.
- `ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>` — Applies the rule engine, emits a deterministic fingerprint, uploads the payload to the IPFS gateway reported by discovery (or the in-memory stub when the gateway is omitted), publishes metadata to JetStream when routes are returned, and prints the CID reported by the gateway.

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
  - Mask: `hash` (SHA-256 truncated), `redact` (literal `REDACTED`), and `last4` (prefixes `last4-` while retaining the final four characters for audit correlation).
  - Synthetic: `uuid` (deterministic `uuid-<table>-<row>` tokens) and `static` (`STATIC`).
- `internal/workflow/snapshots` exposes `LoadDirectory`, `Plan`, and `Capture` helpers for other packages; tests keep coverage ≥90% to honour the critical path status.

Representative fixtures now cover the three primary engines we target locally: Postgres (`dev-db`, `commit-db`), MySQL (`mysql-orders`), and a document store (`doc-events`). Each ships with TOML specs and JSON fixtures under `configs/snapshots/` so `ploy snapshot plan|capture` can exercise real-world rule combinations whether publishing to a live IPFS gateway or the deterministic stub.

## Related Docs
- `docs/design/overview/README.md` — Overall feature architecture.
- `docs/DOCS.md` — documentation matrix and editing conventions.
- `README.md` — High-level CLI overview (snapshot commands included).
- `roadmap/shift/08-documentation-cleanup.md` — slice documenting the doc refresh.
- `cmd/ploy/README.md` — CLI usage details and environment placeholders.
