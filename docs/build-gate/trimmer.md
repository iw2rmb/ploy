# Build Gate Log Trimmer (Maven / Gradle)

This document describes the Build Gate log trimming logic used by the workflow
runtime to keep error output focused while preserving full logs for debugging.

The trimmer runs inside the Build Gate runtime (`internal/workflow/runtime/step`)
and is applied only to **canonical gate logs** produced by the Docker-based
executor (`gate_docker.go`). It is **stack-aware** for Maven and Gradle and
acts as a no-op for all other tools.

## Goals

- Keep **full logs** available for humans and artifacts (for download and
  post-mortem analysis).
- Provide a **focused failure slice** for machine consumers (e.g. Codex healing,
  incident detection, UI summaries).
- Avoid tool/framework-specific heuristics in the node agent or CLI; keep them
  co-located with gate execution.

## Implementation Overview

The trimmer is implemented as:

- `internal/workflow/runtime/step/build_gate_log_trimmer.go`
  - Public entry point:
    - `TrimBuildGateLog(tool, logText string) string`
  - Known `tool` values:
    - `"maven"` — Maven / Surefire logs.
    - `"gradle"` — Gradle test logs.
  - For any other `tool` value, the function returns `logText` unchanged.

### Maven Rules

For Maven (including Surefire + Spring Boot test output), the trimmer:

- Splits logs into lines.
- Anchors on the first line containing `"[ERROR]"`.
- Keeps everything from that line through the end of the log.
- Preserves a trailing newline when the original log ended with one.

With Maven `--ff` enabled in the gate, the first `"[ERROR]"` line typically
corresponds to either:

- A compilation error header (e.g. `COMPILATION ERROR` / `cannot find symbol`), or
- The first failing test summary (e.g. `Tests run: ..., Errors: 1`).

This yields a trimmed view that:

- Drops early plugin/bootstrap noise.
- Keeps the full failure block (summary plus stack trace).
- May include Maven footers (`BUILD FAILURE`, `Total time`) when they appear
  after the first error, which are often useful for context.

### Gradle Rules

For Gradle test logs, the trimmer:

- Splits logs into lines.
- Anchors on:
  - `FAILURE: Build failed with an exception.`
- Preserves a small window of context above the failure header:
  - Up to 20 lines above the anchor (bounded by the start of the file).
- Scans forward and stops after the `BUILD FAILED` summary line:
  - A line starting with `BUILD FAILED in ` or `BUILD FAILED`.
- Joins the selected region back into a string, preserving a trailing newline
  when the original log ended with one.

This yields a trimmed log that typically includes:

- The standard Gradle failure block (`FAILURE: Build failed with an exception.`).
- The key error description (e.g. `Execution failed for task ':sample:test'.`).
- A small amount of surrounding context.
- The `BUILD FAILED` summary but not all preceding task noise.

### Unknown Tools

For any `tool` value other than `"maven"` or `"gradle"`, the trimmer returns
the original log text unchanged. This ensures:

- Non-Java stacks (e.g. Go, ESLint, Error Prone) remain unaffected.
- Future Build Gate adapters can opt into trimming explicitly by setting a
  recognized `Tool` value in `BuildGateStaticCheckReport`.

## Where the Trimmer Is Used

The trimmer is invoked inside the Docker-based gate executor:

- File: `internal/workflow/runtime/step/gate_docker.go`
- When the gate command exits with a non-zero status:
  - `Tool` is set to the detected build tool (e.g. `"maven"`, `"gradle"`, `"go"`,
    `"cargo"`, `"pip"`, `"poetry"`).
  - `TrimBuildGateLog(tool, string(logs))` is called.
  - The trimmed output (or original logs for unknown tools) is stored in
    `BuildGateStageMetadata.LogFindings[0].Message`.
- `BuildGateStageMetadata.LogsText` still carries the full (truncated, ≤10 MiB)
  logs for:
  - Node-side artifact upload (`build-gate.log` bundles).
  - Control-plane storage and manual inspection.

This design keeps the **canonical gate result** (`LogsText`, resource usage,
static checks) intact while also providing a compact, tool-aware error segment
for downstream consumers.

## Healing and Codex Considerations

Healing mods (including `mods-codex`) receive the first failing gate log in
`/in/build-gate.log`. The node agent now prefers the trimmed view when
available:

- When `BuildGateStageMetadata.LogFindings` contains at least one entry, the
  first finding's `Message` is written to `/in/build-gate.log` for healing mods.
- When no trimmed view is available (unknown tool / legacy gate), the agent
  falls back to `BuildGateStageMetadata.LogsText`.

This behavior ensures:

- `mods-codex` and other healing mods see a focused failure slice for known
  tools (Maven/Gradle).
- Full logs remain available via artifacts and `LogsText` for manual inspection.

## Extensibility

To add support for new stacks:

1. Implement a tool-specific trimmer in
   `build_gate_log_trimmer.go` (e.g. `trimGoVetLog`, `trimESLintLog`).
2. Update `TrimBuildGateLog` to dispatch on the new `tool` value.
3. Ensure the corresponding `BuildGateStaticCheckReport.Tool` is set in the
   relevant gate adapter (e.g. `"go-vet"`, `"eslint"`).
4. Add unit tests under
   `internal/workflow/runtime/step/build_gate_log_trimmer_test.go` that cover
   realistic log samples and verify trimming behavior.

The node agent and CLI will automatically benefit from new trimmers via the
existing `LogFindings` and metadata surfaces.
