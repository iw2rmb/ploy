# Build Gate Log Trimmer (Maven / Gradle)

This document describes the Build Gate log trimming logic used by the workflow
runtime to keep error output focused while preserving full logs for debugging.

The trimmer runs inside the Build Gate runtime (`internal/workflow/step`)
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

- `internal/workflow/step/build_gate_log_trimmer.go`
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

For Gradle logs, the trimmer produces:

- A human-focused trimmed message.
- Optional structured evidence payload (`log_findings[0].evidence`) that can be
  forwarded to healing as `/in/errors.yaml`.

Human-focused trimming:

- Splits logs into lines.
- Splits around the first `* What went wrong:` block.
- Preserves compiler diagnostics collected before that block.
- Removes `* Try:` guidance noise from the failure block.
- Deduplicates repeated stack frames and compacts repetition markers.
- Preserves trailing newline parity with input.

Structured evidence modes:

- `compile_java`:
  - Triggered when `* What went wrong:` indicates `Execution failed for task ':compileJava'.`
  - Groups compiler diagnostics by normalized signature (`message` with optional
    `symbol` / `location`) and emits grouped `files[]` references.
  - Excludes Gradle `:compileJava` stacktrace noise from structured payload.
- `plugin_apply`:
  - Triggered when failure block contains `An exception occurred applying plugin request ...`.
  - Extracts `plugin_id` and optional `plugin_version` when present.
  - Keeps concise plugin failure details in `errors[]`.
- `raw`:
  - Fallback for unmatched Gradle failure patterns.
  - Emits concise normalized `* What went wrong:` content.

### Unknown Tools

For any `tool` value other than `"maven"` or `"gradle"`, the trimmer returns
the original log text unchanged. This ensures:

- Non-Java stacks (e.g. Go, ESLint, Error Prone) remain unaffected.
- Future Build Gate adapters can opt into trimming explicitly by setting a
  recognized `Tool` value in `BuildGateStaticCheckReport`.

## Where the Trimmer Is Used

The trimmer is invoked inside the Docker-based gate executor:

- File: `internal/workflow/step/gate_docker.go`
- When the gate command exits with a non-zero status:
  - `Tool` is set to the detected build tool (e.g. `"maven"`, `"gradle"`, `"go"`,
    `"cargo"`, `"pip"`, `"poetry"`).
  - `BuildGateLogFindingContent(tool, string(logs))` computes:
    - trimmed message for `BuildGateStageMetadata.LogFindings[0].Message`
    - optional structured evidence for `BuildGateStageMetadata.LogFindings[0].Evidence`
- `BuildGateStageMetadata.LogsText` still carries the full (truncated, ≤10 MiB)
  logs for:
  - Node-side artifact upload (`build-gate.log` bundles).
  - Control-plane storage and manual inspection.

This design keeps the **canonical gate result** (`LogsText`, resource usage,
static checks) intact while also providing a compact, tool-aware error segment
for downstream consumers.

## Healing and Codex Considerations

Healing migs (including `codex`) receive the first failing gate log in
`/in/build-gate.log`. The node agent now prefers the trimmed view when
available:

- When `BuildGateStageMetadata.LogFindings` contains at least one entry, the
  first finding's `Message` is written to `/in/build-gate.log` for healing migs.
- When `BuildGateStageMetadata.LogFindings[0].Evidence` is present and valid
  YAML/JSON, the node writes `/in/errors.yaml` for heal/re-gate jobs.
- When no trimmed view is available (unknown tool / legacy gate), the agent
  falls back to `BuildGateStageMetadata.LogsText`.

This behavior ensures:

- `codex` and other healing migs see a focused failure slice for known
  tools (Maven/Gradle).
- Structured Gradle failure context can be consumed deterministically through
  `/in/errors.yaml` when available.
- Full logs remain available via artifacts and `LogsText` for manual inspection.

## Extensibility

To add support for new stacks:

1. Implement a tool-specific trimmer in
   `build_gate_log_trimmer.go` (e.g. `trimGoVetLog`, `trimESLintLog`).
2. Update `TrimBuildGateLog` to dispatch on the new `tool` value.
3. Ensure the corresponding `BuildGateStaticCheckReport.Tool` is set in the
   relevant gate adapter (e.g. `"go-vet"`, `"eslint"`).
4. Add unit tests under
   `internal/workflow/step/build_gate_log_trimmer_test.go` that cover
   realistic log samples and verify trimming behavior.

The node agent and CLI will automatically benefit from new trimmers via the
existing `LogFindings` and metadata surfaces.
