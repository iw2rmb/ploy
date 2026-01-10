# Contract: Spec Merge Strictness (Server)

This file tracks *remaining* spec-merge contract work that is not yet reflected
in `docs/` at HEAD.

## Goals

- When merging spec JSON, reject invalid JSON and non-object JSON at request
  boundaries with a 400.
- Do not silently substitute `{}` for invalid inputs.
- When spec JSON is sourced from the DB and violates expected shape (non-object),
  treat as a server invariant violation (500).

## Notes / Touch Points

- Likely touch points:
  - `internal/server/handlers/spec_utils.go`
  - `internal/server/handlers/nodes_claim.go` (merge helpers injecting `job_id`,
    `mod_index`, and similar fields)

