# Stack Gate — Phase 5: Expand Detectors (Go/Rust/Python) + UX

Scope: Extend `stackdetect` beyond Java and improve Stack Gate failure visibility (expected vs detected + evidence) without leaking file contents.

Documentation: `design/stack-gate.md`, `internal/workflow/stackdetect` (Phase 2), `docs/mods-lifecycle.md` (gate/healing semantics).

Legend: [ ] todo, [x] done.

## Go detection
- [ ] Detect Go release from `go.mod` — Enables stack-aware gating for Go toolchain bumps.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Parse `go.mod` for `go 1.xx` (canonicalize to `"1.xx"`); optionally record `toolchain go1.xx` as evidence.
  - Snippets: Evidence `{path:"go.mod", key:"go", value:"1.22"}`
  - Tests: `go test ./internal/workflow/stackdetect -run GoMod` — valid `go.mod` yields deterministic release.

## Rust detection
- [ ] Detect Rust release from `rust-toolchain*` and `Cargo.toml` — Enables MSRV gating.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Prefer `Cargo.toml` `rust-version`; otherwise parse `rust-toolchain(.toml)` numeric channel; treat `stable`/`nightly` as unknown for release matching.
  - Snippets: Evidence `{path:"Cargo.toml", key:"rust-version", value:"1.76"}`
  - Tests: `go test ./internal/workflow/stackdetect -run Rust` — numeric channels pass; stable/nightly classify unknown.

## Python detection
- [ ] Detect Python release from `pyproject.toml`/`.python-version`/`runtime.txt` — Enables deterministic gating for Python minor bumps.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Canonicalize exact versions `3.11.6` → `"3.11"`; accept only reducible specifiers (e.g. `>=3.11,<3.12`); disagreements across sources → unknown.
  - Snippets: Evidence `{path:".python-version", key:"python", value:"3.11"}`
  - Tests: `go test ./internal/workflow/stackdetect -run Python` — reducible specifiers pass; spanning ranges classify unknown.

## UX and observability
- [ ] Surface Stack Gate mismatch/unknown with evidence — Makes failures actionable without reading full build logs.
  - Repository: ploy
  - Component: nodeagent + CLI output (where gate results are rendered)
  - Scope: Ensure gate failure output includes phase, expected vs detected, and evidence `{path,key,value}` only.
  - Snippets: N/A
  - Tests: `go test ./... -run StackGate.*Output` — output contains expected/detected and evidence; no file contents.
