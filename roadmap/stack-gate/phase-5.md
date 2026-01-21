# Stack Gate — Phase 5: Expand Detectors (Go/Rust/Python) + UX

Status: **Planned (not implemented)**

## Goal

Add non-Java stack detection and tighten UX/observability for Stack Gate failures.

## What remains unchanged

- Healing semantics remain unchanged except that Stack Gate mismatch/unknown are treated as policy failures (no healing) once Phase 4 is active.

## Compatibility impact

- None required (new detectors extend supported stacks; strictness is controlled by `stack.*.enabled`).

## Implementation steps (RED → GREEN → REFACTOR)

1. Add Go detection:
   - `go.mod` parsing for `go 1.xx` (and optional `toolchain go1.xx`) → release `"1.xx"`.
2. Add Rust detection:
   - Parse `rust-toolchain.toml` / `rust-toolchain` and `Cargo.toml` (`rust-version` preferred).
   - Treat `stable`/`nightly` channels as unknown for release matching.
3. Add Python detection:
   - Parse `pyproject.toml` (`requires-python` or Poetry python constraint), `.python-version`, `runtime.txt`.
   - Canonicalize exact `3.11.6` → `"3.11"`, and reject non-reducible ranges as unknown.
4. UX + metadata polish:
   - Ensure CLI/log output includes expected vs detected plus evidence keys/paths (no file contents).
5. Tests:
   - Add fixtures + unit tests for each language under `internal/workflow/stackdetect/`.

