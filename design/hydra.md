# Hydra: Unified Env and File Materialization Contract

## Summary

Introduce one explicit contract for classic environment variables and file materialization in MIG specs:

- `envs` for key/value environment variables only.
- `ca` for CA materialization.
- `in` for read-only materialized inputs under `/in`.
- `out` for read-write materialized paths under `/out` (with final `/out` upload unchanged).
- `home` for materialized files under `$HOME` (destination is always `$HOME`-relative).

All file-backed values use content-addressed upload and deterministic materialization. Environment variables carry only scalar key/value pairs — no multiline payloads, no file inlining, no shell preamble injection.

## Scope

In scope:

- MIG spec contract changes (`envs`, `ca`, `in`, `out`, `home`) for step, router, and healing containers.
- CLI compile-time normalization and upload of local file/folder inputs.
- Server claim-time defaults overlay for the new fields.
- Node runtime materialization and mount planning for the new fields.
- `ploy config {env|ca|home} {set|unset|ls}` global configuration commands.
- `$PLOY_CONFIG_HOME/config.yaml` support with simplified target sections:
  - `server`, `node`, `pre_gate`, `re_gate`, `post_gate`, `mig`.

Out of scope:

- Changes to run orchestration semantics (job graph, retries, gate policy).

## Motivation

Hydra consolidates file delivery under one contract to enforce strict separation of concerns:

- Environment variables carry only scalar key/value pairs.
- File-backed values are content-addressed, uploaded once, and materialized at deterministic paths.
- CA delivery uses file materialization — no env injection or shell preambles.
- Mount domains (`/in`, `/out`, `$HOME`) constrain destination rules by field type.

## Goals

- Strict separation: env values are scalar key/value pairs; files are content-addressed materialized resources.
- Deterministic, content-addressed file delivery via single compile/upload/materialize pipeline.
- Explicit and deterministic merge precedence (server defaults < local config < spec).
- Mount behavior constrained by destination domain (`/in` read-only, `/out` read-write, `$HOME` configurable).
- Shared bundle upload/download/verify/extract infrastructure across all materialization fields.
- One contract, one code path — no parallel or special-case flows.

## Non-goals

- Adding new job-type selector families beyond the simplified `config.yaml` sections.
- Supporting hybrid mode where multiple materialization contracts coexist.

## Architecture

### Key implementation surfaces

- CLI compile path: `cmd/ploy/mig_run_spec.go`, `cmd/ploy/mig_run_spec_bundle.go`
- Contracts: `internal/workflow/contracts/migs_spec.go`, `internal/workflow/contracts/build_gate_config.go`
- Claim-time mutator pipeline: `internal/server/handlers/claim_spec_mutator_base.go`, `internal/server/handlers/claim_spec_mutator_pipeline.go`
- Node materialization: `internal/nodeagent/execution_orchestrator_bundle.go`, `internal/nodeagent/execution_orchestrator_jobs.go`
- Container spec: `internal/workflow/step/container_spec.go`
- `/out` upload: `internal/nodeagent/execution_orchestrator_jobs_upload.go`
- CA materialization: `internal/workflow/step/gate_command_materializer.go`, `internal/workflow/step/gate_plan_command_selector.go`
- Config: `internal/cli/config/config.go`, `internal/store/schema.sql`, `internal/server/handlers/config_env.go`, `cmd/ploy/config_env_command.go`

### Contract

### 1. Canonical fields

Canonical fields:

- `envs: map[string]string`
- `ca: []string`
- `in: []string`
- `out: []string`
- `home: []string`

Authoring input formats (local spec + local config overlays before compile):

- `in`: `src:dst`
- `out`: `src:dst`
- `home`: `src:dst{:ro}`

Canonical stored spec format (after compile/upload):

- General form: `shortHash:dst{:mo}`.
- In current contract, `mo` is used only for `home` and only `ro` is allowed.
- `in`: `shortHash:dst`
- `out`: `shortHash:dst`
- `home`: `shortHash:dst{:ro}`

### 2. Destination domain rules

- `in`:
  - `dst` must be absolute and start with `/in/`.
  - mode is fixed to `ro` by contract and cannot be overridden in spec.
- `out`:
  - `dst` must be absolute and start with `/out/`.
  - mode is fixed to `rw` by contract and cannot be overridden in spec.
  - existing full `/out` artifact upload remains.
- `home`:
  - `dst` is always relative to `$HOME`.
  - absolute paths and `..` traversal are rejected.
  - final mount target is `$HOME/<dst>`.
  - mode defaults to `rw`; optional `:ro` forces read-only.
- `ca`:
  - source paths are materialized as CA inputs and mounted at deterministic runtime CA path(s).
  - no CA inline payload in envs.

### 3. Parser rules

`in` and `out`:

1. Split at the last `:`.
2. Left part is `src`, right part is `dst`.
3. No explicit mode suffix is accepted.

`home`:

1. Parse as `src:dst{:ro}`.
2. If `:ro` suffix is present, mode is read-only.
3. If `:ro` suffix is absent, mode is read-write.
4. Split source/destination using right-biased parsing on the remaining prefix.

This preserves familiar docker-like source/destination notation while making `in/out` mode semantics strict by contract.

Canonical parser rules for stored spec entries:

- Parse `shortHash:dst` (`in`, `out`) and `shortHash:dst{:ro}` (`home`).
- `shortHash` is hex-like and `:`-free by contract, which minimizes delimiter ambiguity.

### 4. Compilation and canonicalization model

For each `ca`/`in`/`out`/`home` source record supplied by spec or local config:

1. Resolve path relative to spec/config file location.
2. Build deterministic tar.gz payload.
3. Compute a git-style content hash for the payload (object-style preimage + cryptographic digest).
4. Resolve existing object by hash before upload.
5. Upload only when hash is not found.
6. Rewrite to canonical hash-first entries in submitted spec (`shortHash:dst{:ro}` shape by field).

Canonical spec string form uses hash-first entries; control-plane metadata resolves hashes to full typed object identity for runtime materialization.

Dedup/reuse contract:

- Identical payload bytes must resolve to the same content hash and same canonical object identity.
- If the hash already exists in control-plane metadata/object store, the compiler reuses the existing reference and skips blob upload.
- Upload endpoint behavior remains idempotent: repeated uploads of identical bytes return existing bundle metadata and do not create new object blobs.
- This rule applies uniformly to `ca`, `in`, `out`, and `home` materialized records.

Short-hash contract:

- `shortHash` is a deterministic prefix of the full content hash.
- Prefix length is fixed by contract and extended when needed to avoid collisions.
- Control plane resolves `shortHash` to full canonical object identity before runtime materialization.

### 5. Overlay precedence and ownership

Effective per-job configuration precedence:

1. Server defaults (control-plane).
2. Local `$PLOY_CONFIG_HOME/config.yaml`.
3. Spec file values.

Per-field merge rules:

- `envs`: key-based override by precedence.
- `ca`: append by precedence with dedup by digest.
- `in/out/home`: merge by destination path; higher precedence replaces same destination record.

Job section routing for `config.yaml` and server defaults:

- `pre_gate` applies to pre-gate job containers.
- `re_gate` applies to re-gate job containers.
- `post_gate` applies to post-gate job containers.
- `mig` applies to mig job containers.
- `heal` applies to heal job containers.
- Router containers inherit the active gate phase section (`pre_gate`, `re_gate`, or `post_gate`).

### 6. Simplified `config.yaml` shape

`$PLOY_CONFIG_HOME/config.yaml` supports:

```yaml
---
defaults:
  server:
    envs: {}
    ca: []
    home: []
  
  node:
    envs: {}
    ca: []
    home: []
  
  job:
    pre_gate:
      envs: {}
      ca: []
      in: []
      out: []
      home: []
    
    re_gate:
      envs: {}
      ca: []
      in: []
      out: []
      home: []
    
    post_gate:
      envs: {}
      ca: []
      in: []
      out: []
      home: []
    
    mig:
      envs: {}
      ca: []
      in: []
      out: []
      home: []
      
    heal:
      envs: {}
      ca: []
      in: []
      out: []
      home: []
```

`server` and `node` sections are component-scoped; job sections are claim/runtime-scoped.

Schema distribution contract:

- Repository source: `cmd/ploy/assets/config.schema.json`.
- CLI embed: schema is embedded into `ploy` binary assets.
- Deploy behavior: `ploy cluster deploy` writes
  `$PLOY_CONFIG_HOME/config.schema.json` from embedded bytes.

### 7. Unsupported fields

Only the canonical Hydra fields (`envs`, `ca`, `in`, `out`, `home`) are accepted for environment and file materialization. Any other materialization fields are rejected at schema validation.

## Implementation Notes

### Contracts

- MIG contracts:
  - `internal/workflow/contracts/migs_spec.go`
  - `internal/workflow/contracts/build_gate_config.go`
- Typed materialization mount records in `StepManifest`/container planning:
  - `internal/workflow/contracts/step_manifest.go`
  - `internal/workflow/step/container_spec.go`

### CLI compile path

- Single compiler stage:
  - loads `$PLOY_CONFIG_HOME/config.yaml`,
  - applies local overlay,
  - validates path/domain rules,
  - uploads all file-backed records,
  - emits canonical spec JSON.
- Files:
  - `cmd/ploy/mig_run_spec.go`
  - `cmd/ploy/mig_run_spec_bundle.go`

### Server overlay

- Claim mutator pipeline performs typed merge for `envs/ca/in/out/home`.
- Deterministic ordering with one parse + one marshal architecture.
- Files:
  - `internal/server/handlers/claim_spec_mutator_base.go`
  - `internal/server/handlers/claim_spec_mutator_pipeline.go`
  - `internal/server/handlers/nodes_claim_response.go`

### Node runtime

- Generic materialized-resource staging for all fields.
- Mount plan built from canonical records across `/in`, `/out`, `$HOME`, and CA locations.
- Digest verification and traversal-safe extraction for all materialized resources.
- Files:
  - `internal/nodeagent/execution_orchestrator_bundle.go` (promote to generic materializer module)
  - `internal/nodeagent/execution_orchestrator_jobs.go`
  - `internal/workflow/step/runner.go`
  - `internal/workflow/step/container_spec.go`

### Config persistence and commands

- Typed config storage per target section.
- Commands:
  - `ploy config env {set|unset|ls}`
  - `ploy config ca {set|unset|ls}`
  - `ploy config home {set|unset|ls}`
- Files:
  - `cmd/ploy/config_command.go`
  - `cmd/ploy/config_env_command.go` (split/generalize)
  - `internal/server/handlers/config_env.go` (replace/generalize)
  - `internal/store/schema.sql`
  - `internal/store/queries/*`

Hydra field mapping (final state):

| Hydra field | Materialization target | Example |
|---|---|---|
| `envs` | Key/value environment variables (scalar only) | `envs: {"OPENAI_API_KEY": "sk-..."}` |
| `ca` | CA certificate materialization via file delivery | `ca: ["<hash>"]` |
| `in` | Read-only inputs under `/in` | `in: ["<hash>:/in/codex-prompt.txt"]` |
| `out` | Read-write outputs under `/out` | `out: ["<hash>:/out/result.json"]` |
| `home` | Files under `$HOME` (default rw, optional `:ro`) | `home: ["<hash>:.codex/auth.json:ro"]` |

All file-backed values use content-addressed upload. No env key carries file content.
Only the canonical Hydra fields above are accepted; any other materialization fields are rejected at validation.

## Milestones

### Milestone 1: Contract types and validation

Scope:

- Spec fields and validators for `envs`, `ca`, `in`, `out`, `home`.

Expected results:

- Specs containing unsupported fields fail validation.
- Specs with Hydra fields parse and validate deterministically.

Testable outcome:

- Contract/unit tests cover parser behavior, path-domain rules, and merge conflict detection.

### Milestone 2: CLI compiler and local overlay

Scope:

- Implement unified compiler for file-backed records.
- Load and merge `$PLOY_CONFIG_HOME/config.yaml` sections.

Expected results:

- Submitted specs contain canonical uploaded refs.
- Local config merges before submission with correct precedence.

Testable outcome:

- CLI tests verify path resolution, strict `in/out` parsing (`src:dst` only), `home` parsing (`src:dst{:ro}`), upload dedup semantics, and precedence.

### Milestone 3: Claim-time server defaults overlay

Scope:

- Extend mutator pipeline to typed defaults overlay.
- Persist/retrieve new global config shape for target sections.

Expected results:

- Effective claim spec honors server < local < spec precedence.
- Ordering and overwrite behavior are deterministic.

Testable outcome:

- Handler tests for merge precedence and target routing by job type.

### Milestone 4: Node materialization and mount runtime

Scope:

- Generic resource materialization and mount plan execution for all Hydra fields.

Expected results:

- Containers receive correct `/in`, `/out`, `$HOME`, and CA materializations.
- File-backed values are delivered exclusively via materialization, not env injection.

Testable outcome:

- Node/workflow tests validate extraction safety, mount targets, mode enforcement, and `/out` upload continuity.

## Acceptance Criteria

- Only Hydra fields (`envs`, `ca`, `in`, `out`, `home`) are accepted; legacy fields are forbidden at validation.
- Hydra fields validate with deterministic error messages for invalid destination/mode/domain.
- Stored spec entries use canonical hash-first format (`shortHash:dst{:ro}` per field rules).
- Effective configuration precedence is exactly: server defaults < local config.yaml < spec.
- `in` mounts are read-only under `/in`; `out` mounts are read-write under `/out`; `home` mounts are under `$HOME`.
- `home` mode semantics are deterministic: default `rw`, optional `:ro`.
- CA is delivered via file materialization, not multiline env injection.
- Materialization logic uses shared compile/runtime pipelines with one code path per field type.
- Existing `/out` final artifact upload behavior remains functional.
- Re-submitting identical file/folder content must reuse existing CAS objects and must not create additional object-store blobs.

## Notes

- `src:dst` and `src:dst:ro` notation follows Docker bind-mount style.
- Canonical `shortHash:dst{:ro}` format avoids delimiter ambiguity because hash tokens do not contain `:`.

## References

- `cmd/ploy/mig_run_spec.go`
- `cmd/ploy/mig_run_spec_bundle.go`
- `internal/workflow/contracts/migs_spec.go`
- `internal/workflow/contracts/build_gate_config.go`
- `internal/workflow/contracts/step_manifest.go`
- `internal/workflow/step/container_spec.go`
- `internal/workflow/step/gate_command_materializer.go`
- `internal/server/handlers/claim_spec_mutator_base.go`
- `internal/server/handlers/claim_spec_mutator_pipeline.go`
- `internal/server/handlers/config_env.go`
- `images/codex/entrypoint.sh`
- `images/amata/entrypoint.sh`
- `images/server/entrypoint.sh`
- `images/node/entrypoint.sh`
- `images/orw/orw-cli-gradle/orw-cli.sh`
- `images/orw/orw-cli-maven/orw-cli.sh`
- `internal/nodeagent/execution_orchestrator_jobs.go`
- `internal/nodeagent/execution_orchestrator_bundle.go`
- `internal/store/schema.sql`
