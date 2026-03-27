# Dependency Bumps Compatibility Loop

## Goal

Automate dependency-version healing by:

- carrying cumulative dependency bumps through the healing loop,
- storing SBOMs from successful gate runs,
- exposing compatibility hints from stored SBOMs for `deps` healing.

## Scope

- `deps` healing LLM output contract.
- Cumulative `bumps` merge in gate/heal `jobs.meta.recovery`.
- Successful gate SBOM persistence.
- Compatibility endpoint based on stored SBOM rows.
- Inputs injected into `deps` healing claims.

## Out Of Scope

- Backward compatibility aliases for field names.
- Historical migration of old metadata payloads.
- Compatibility inference from failed gates.

## Terms

- `delta_bumps`: version changes produced by the current healing attempt.
- `deps_bumps`: cumulative version state across loop iterations.
- `disable`: library value `null`, meaning remove/disable that dependency.

## `deps` Healing Output Contract

Healing strategy writes one JSON object to `/out/codex-last.txt`:

```json
{
  "summary": "Updated Spring and disabled legacy guava shim.",
  "bumps": {
    "org.springframework:spring-core": "6.1.8",
    "com.google.guava:guava-shim": null
  }
}
```

Rules:

- Canonical field is `bumps` (not `bumbs`).
- Keys are normalized dependency names.
- Value is either non-empty version string or `null`.
- Empty map is allowed.
- Unknown top-level keys are ignored.

## Recovery Metadata Extensions

Extend `BuildGateRecoveryMetadata` with:

- `deps_bumps` object (`map[string]*string`)

Semantics:

- `deps_bumps` is cumulative state used by the next `deps` attempt.
- Healing summary remains in existing `jobs.meta.action_summary`.

## Merge Algorithm

At healing insertion (`failed gate -> heal -> re_gate`):

- Start from failed gate `recovery.deps_bumps` (or `{}`).
- Copy into both created jobs (`heal` and `re_gate`) as `deps_bumps`.

On successful `heal` completion:

- Parse `bumps` from `/out/codex-last.txt`.
- Merge with prior `deps_bumps` using last-write-wins by key.
- Persist merged map into linked `re_gate` job metadata.

Merge rule:

- `merged[k] = delta[k]` when key exists in delta.
- Otherwise keep previous value.
- `null` is a valid terminal value and must be preserved.

## Gate Success SBOM Persistence

SBOM ingestion runs only for successful gate jobs (`pre_gate`, `post_gate`, `re_gate`):

- Detect SBOM artifacts from uploaded `/out/*`.
- Parse supported SBOM formats.
- Flatten packages to normalized `(lib, ver)` rows.
- Persist rows keyed by `job_id` and `repo_id`; stack/time are resolved by joins.

`design/sboms.md` remains the base artifact flow; this document extends it with compatibility query needs.

## SBOM Data Model

Use `design/sboms.md` table shape:

- `sboms(job_id, repo_id, lib, ver)`

No duplicate stack/date columns are required:

- Time is available from `sboms.job_id -> jobs.id -> jobs.created_at`.
- Stack is available from `sboms.job_id -> gates.job_id -> gate_profiles.stack_id -> stacks`.

## Compatibility API

Endpoint:

- `GET /v1/sboms/compat?lang=<lang>&release=<release>&tool=<tool>&libs=<name>:<ver>,<name>`

Behavior:

- Resolve stack filter via joins:
  - `sboms.job_id -> gates -> gate_profiles -> stacks`
- Select only successful gate jobs via `sboms.job_id -> jobs`.
- For each requested lib:
  - `name`: return minimum observed successful version.
  - `name:ver`: return minimum observed successful version that is `>= ver`.
- Return object: `{ "<lib>": "<ver>", ... }`.
- Return `null` when stack has no successful SBOM evidence.

Version comparison must be ecosystem-aware, not lexical.

## `deps` Healing Inputs

For `heal` claims where `selected_error_kind=deps`, server provides:

- `recovery_context.deps_bumps` from the last failed gate metadata.
- `recovery_context.deps_compat_endpoint` template:
  - `/v1/sboms/compat?lang=<...>&release=<...>&tool=<...>&libs=...`
- `recovery_context.detected_stack` (already present) as source of `lang/release/tool`.

Nodeagent hydrates:

- `/in/deps-bumps.json` with `deps_bumps`.
- `/in/deps-compat-url.txt` with stack-prefilled endpoint.

## Runtime Flow

1. Gate fails with `error_kind=deps`.
2. Server inserts `heal -> re_gate`, carrying `deps_bumps`.
3. `deps` heal receives previous bumps + compat endpoint, emits `summary` + `bumps`.
4. Server merges returned `bumps` into cumulative `deps_bumps` for next `re_gate`.
5. If `re_gate` succeeds, gate stores full successful SBOM rows.
6. Next `deps` failures query `/v1/sboms/compat` for evidence-backed minimums.

## Required Code Changes

- Contracts:
  - `internal/workflow/contracts/build_gate_metadata.go`
- Nodeagent parsing and heal metadata:
  - `internal/nodeagent/recovery_io.go`
  - `internal/nodeagent/execution_orchestrator_jobs.go`
  - `internal/nodeagent/execution_orchestrator_healing_runtime.go`
- Healing insertion and merge:
  - `internal/server/handlers/nodes_complete_healing.go`
  - `internal/server/handlers/jobs_complete.go`
  - `internal/server/handlers/nodes_claim.go`
- SBOM persistence and compat API:
  - `internal/store/schema.sql`
  - `internal/store/queries/*.sql` (new sbom queries)
  - `internal/server/handlers/*` (new `/v1/sboms/compat` handler)

## Validation

- Unit tests for `deps` output parsing (`summary`, `bumps`, `null` disable).
- Unit tests for `deps_bumps` merge semantics.
- Claim response tests for `deps`-specific recovery context fields.
- SBOM ingestion tests: only successful gates persist rows.
- Compat API tests: object response, `null` response, version-floor filtering.

## Related Docs

- `design/sboms.md`
- `docs/migs-lifecycle.md`
- `docs/build-gate/README.md`
