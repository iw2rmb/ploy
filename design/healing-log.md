# Healing Log (build-gate ↔ healing iterations)

Status: draft (design only)

## Problem

The healing loop currently has no durable, structured record of:

- What the build gate failure was (as a single, human-readable one-liner).
- What each healing attempt did (as a single, human-readable one-liner).
- Which exact build-gate logs and agent logs correspond to each retry iteration.

This makes debugging, auditing, and UX (“follow”/UIs) harder.

## Requirements (as-is)

When running the healing process:

1. **Prep (before healing):** execute a dedicated “router” coding agent job that reads `/in/build-gate.log` and emits a **Bug Summary** one-liner; persist that one-liner into the **Job record** for the corresponding **build-gate job**.
2. **After healing completes:** obtain an **Action Summary** one-liner from the healing step and store it in the corresponding **healing job record**.
3. **For every healing step (iteration):** write `/in/healing-log.md` containing a list of one-liners and the paths to the build-gate log and the healing agent log, in this format:

```md
# Healing Log

## Iteration 1

- Bug Summary: <one-liner>
  Build Log: /in/build-gate-iteration-1.log
- Healing Attempt: <one-liner>
  Agent Log: /in/healing-iteration-1.log

...

## Iteration X:
```

Iterations count for the build-gate ↔ healing cycle.

## Definitions

- **Iteration N:** one build-gate failure that triggers healing, plus the healing attempt that follows it.
- **Bug Summary (one-liner):** a single line describing the build-gate failure (source: `/in/build-gate.log` content at that iteration, summarized by `build_gate.router`).
- **Healing Attempt (one-liner):** a single line describing what the healer changed/did for that iteration (reported by the healing mod).

## Artifacts (files under healing job `/in`)

Per healing loop, the node creates and maintains:

- `/in/healing-log.md` — the running summary (append/update per iteration).
- `/in/build-gate.log` — “latest” build-gate failure context (already established contract).
- `/in/build-gate-iteration-N.log` — the captured build-gate failure log for iteration `N`.
- `/in/healing-iteration-N.log` — the captured healer/agent log for iteration `N`.

Notes:

- `build-gate-iteration-N.log` and `healing-iteration-N.log` are the canonical per-iteration records referenced by `/in/healing-log.md`.
- `/in/build-gate.log` may remain as a convenience alias to the latest failing gate log; it is not sufficient for audit/history.

## Job storage (where the one-liners live)

The requirement says “store in Job record”. In this repo, the durable job record extension point is `jobs.meta` (JSONB), shaped by `internal/workflow/contracts.JobMeta`.

Proposed storage (additive fields):

- **Gate job** (kind=`"gate"`): store Bug Summary on the gate metadata as `jobs.meta.gate.bug_summary`.
- **Healing job** (kind=`"mod"`, mod_type=`"healing"`): store Action Summary as `jobs.meta.action_summary`.

Example (gate job):

```json
{
  "kind": "gate",
  "gate": {
    "bug_summary": "javac: cannot find symbol FooBar in src/main/java/...",
    "static_checks": [],
    "log_findings": []
  }
}
```

Example (healing job):

```json
{
  "kind": "mod",
  "mods_step_name": "heal",
  "action_summary": "Updated import path to FooBar; regenerated sources."
}
```

## One-liner generation contracts

### Bug Summary (prep, via `build_gate.router`)

Input:

- `/in/build-gate-iteration-N.log` (same content as `/in/build-gate.log` at time of iteration `N`).

Output:

- A single line (no newlines) suitable for UI display and job metadata.

Mechanism:

- Run the router coding agent image (default: Codex mod) with a prompt that:
  - reads `/in/build-gate.log`
  - outputs a single-line JSON object as the final message:
    - `{"bug_summary":"..."}`

Constraints:

- Trim whitespace.
- Max length: 200 chars (truncate with `…`).

### Action Summary (post-heal, from healing step)

Input:

- The healing step final message (Codex final output), captured by the node from the healer agent output.

Output:

- A single line (no newlines) describing what was done.

Mechanism:

- Require the healing mod (Codex) to output a single-line JSON object as the final message:
  - `{"action_summary":"..."}`

Constraints:

- Trim whitespace.
- Max length: 200 chars (truncate with `…`).

## Healing log write semantics

For each iteration `N`:

1. Capture failing build-gate log into `/in/build-gate-iteration-N.log` (and update `/in/build-gate.log` as “latest”).
2. Execute `build_gate.router`, parse `bug_summary`, and persist it into the corresponding **gate job** record.
3. Ensure `/in/healing-iteration-N.log` is written with the healer/agent logs for that attempt.
4. Parse `action_summary` from the healing step final output and persist it into the corresponding **healing job** record.
5. Append/update `/in/healing-log.md` with the two one-liners and the two paths (exact format above).

## Implementation notes (follow-up work)

This design implies code changes (not included here):

- Extend `internal/workflow/contracts.BuildGateStageMetadata` (or `JobMeta`) to carry `bug_summary`.
- Extend `internal/workflow/contracts.JobMeta` to carry `action_summary` (valid for `kind="mod"`).
- Update the Mods spec schema:
  - Move `build_gate.healing.mod.*` fields up to `build_gate.healing.*` (remove the `mod` object).
  - Add `build_gate.router.*` for the router coding-agent job (image + prompt/env).
- In the node healing loop (`internal/nodeagent/execution_healing.go`), run the router before healing, write the per-iteration files, and update `jobs.meta` for the gate and healing jobs.
