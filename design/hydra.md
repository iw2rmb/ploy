# Hydra: Unified Env and File Materialization Contract

## Summary

Introduce one explicit contract for classic environment variables and file materialization in MIG specs:

- `envs` for key/value environment variables only.
- `ca` for CA materialization.
- `in` for read-only materialized inputs under `/in`.
- `out` for read-write materialized paths under `/out` (with final `/out` upload unchanged).
- `home` for materialized files under `$HOME` (destination is always `$HOME`-relative).

This removes shell-fragile multiline env transport and replaces legacy `env_from_file`, `tmp_dir`, `tmp_bundle`, and `PLOY_CA_CERTS`-style behavior with one deterministic path.

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

- Backward compatibility for removed spec fields and commands.
- Hybrid mode where old and new contracts coexist.
- Changes to run orchestration semantics (job graph, retries, gate policy).

## Why This Is Needed

Current file delivery paths are fragmented and leak transport concerns into runtime behavior:

- `env_from_file` inlines file contents into env values at CLI preprocessing time.
- `tmp_dir` is rewritten to uploaded `tmp_bundle`.
- CA delivery relies on env materializer shell preambles.
- Multiline env values are known to break `/bin/sh` export handling.

The same codebase already has strong primitives for content-addressed upload and node-side rehydration/materialization, but they are not unified under one clear contract.

## Goals

- Separate concerns: env values are env values; files are files.
- Make file delivery deterministic and content-addressed.
- Keep merge precedence explicit and deterministic.
- Keep runtime mount behavior explicit and constrained by destination domains.
- Reuse existing bundle upload/download/verify/extract infrastructure.
- Consolidate fragmented materialization paths into shared components with less duplicated logic.
- Reduce implementation complexity by replacing special-case flows with one strict contract.
- Remove legacy paths in one hard cut.

## Non-goals

- Maintaining deprecated fields as aliases.
- Preserving legacy CLI UX (`env_from_file`, `tmp_dir`, `tmp_bundle`, `PLOY_CA_CERTS` env materialization).
- Adding new job-type selector families beyond the requested simplified `config.yaml` sections.

## Current Baseline (Observed)

- CLI preprocesses `env_from_file` and rewrites `tmp_dir` into uploaded `tmp_bundle`:
  - `cmd/ploy/mig_run_spec.go`
  - `cmd/ploy/mig_run_spec_tmpbundle.go`
- Contracts still expose `env` and `tmp_bundle` and explicitly reject legacy `tmp_dir`:
  - `internal/workflow/contracts/migs_spec.go`
  - `internal/workflow/contracts/build_gate_config.go`
- Claim-time mutator pipeline merges global env into `spec.env` only:
  - `internal/server/handlers/claim_spec_mutator_base.go`
  - `internal/server/handlers/claim_spec_mutator_pipeline.go`
- Node runtime materializes tmp bundles and mounts `/tmp/<entry>`:
  - `internal/nodeagent/execution_orchestrator_tmpbundle.go`
  - `internal/nodeagent/execution_orchestrator_jobs.go`
  - `internal/workflow/step/container_spec.go`
- `/out` is already a dedicated writable mount uploaded as artifact bundle:
  - `internal/nodeagent/execution_orchestrator_jobs_upload.go`
- CA handling currently depends on env preambles (`PLOY_CA_CERTS`) and shell execution:
  - `internal/workflow/step/gate_command_materializer.go`
  - `internal/workflow/step/gate_plan_command_selector.go`
- Codex image entrypoint already documents multiline env export fragility:
  - `images/codex/entrypoint.sh`
- CLI config home exists, but no shared `config.yaml` runtime overlay loader exists:
  - `internal/cli/config/config.go`
- Global config persistence is env-only today:
  - `internal/store/schema.sql` (`config_env`)
  - `internal/server/handlers/config_env.go`
  - `cmd/ploy/config_env_command.go`

## Target Contract or Target Architecture

### 1. Canonical fields

Replace legacy fields with:

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

### 7. Hard-cut removals

Remove support for:

- `env` and `env_from_file`.
- `tmp_dir` and `tmp_bundle`.
- `PLOY_CA_CERTS` env-based materialization path.
- legacy config env-only command surface as the primary config model.

## Implementation Notes

### Contracts

- Update raw MIG contracts in:
  - `internal/workflow/contracts/migs_spec.go`
  - `internal/workflow/contracts/build_gate_config.go`
- Replace tmp-bundle-centric fields in `StepManifest`/container planning with typed materialization mount records:
  - `internal/workflow/contracts/step_manifest.go`
  - `internal/workflow/step/container_spec.go`

### CLI compile path

- Replace current preprocessors:
  - `resolveEnvFromFileInPlace`
  - `archiveAndUploadTmpDirsInPlace`
- Add one compiler stage that:
  - loads `$PLOY_CONFIG_HOME/config.yaml`,
  - applies local overlay,
  - validates path/domain rules,
  - uploads all file-backed records,
  - emits canonical spec JSON.
- Files:
  - `cmd/ploy/mig_run_spec.go`
  - `cmd/ploy/mig_run_spec_tmpbundle.go` (rename/repurpose to generic bundle compiler).

### Server overlay

- Extend claim mutator pipeline from env-only merge to typed merge for `envs/ca/in/out/home`.
- Keep deterministic ordering and one parse + one marshal architecture.
- Files:
  - `internal/server/handlers/claim_spec_mutator_base.go`
  - `internal/server/handlers/claim_spec_mutator_pipeline.go`
  - `internal/server/handlers/nodes_claim_response.go`

### Node runtime

- Replace `withMaterializedTmpBundle` path with generic materialized-resource staging.
- Build mount plan from canonical records across `/in`, `/out`, `$HOME`, and CA locations.
- Continue using existing digest verification and traversal-safe extraction logic.
- Files:
  - `internal/nodeagent/execution_orchestrator_tmpbundle.go` (promote to generic materializer module)
  - `internal/nodeagent/execution_orchestrator_jobs.go`
  - `internal/workflow/step/runner.go`
  - `internal/workflow/step/container_spec.go`

### Config persistence and commands

- Replace env-only global config persistence with typed config storage per target section.
- Implement:
  - `ploy config env {set|unset|ls}`
  - `ploy config ca {set|unset|ls}`
  - `ploy config home {set|unset|ls}`
- Files:
  - `cmd/ploy/config_command.go`
  - `cmd/ploy/config_env_command.go` (split/generalize)
  - `internal/server/handlers/config_env.go` (replace/generalize)
  - `internal/store/schema.sql`
  - `internal/store/queries/*`

Special env migration table (current env-only admin workflows -> typed fields):

| Legacy env key | Current special behavior | Target typed field | Target mapping |
|---|---|---|---|
| `PLOY_CA_CERTS` | Materialized by shell/env logic in server/node/gate/ORW runtimes | `ca` | `ca: [<local-ca-path>]` |
| `CODEX_AUTH_JSON` | Materialized by codex/amata entrypoints into Codex auth file | `home` | `home: ["<src>:.codex/auth.json:ro"]` |
| `CODEX_CONFIG_TOML` | Materialized by codex/amata entrypoints into Codex config file | `home` | `home: ["<src>:.codex/config.toml:ro"]` |
| `CRUSH_JSON` | Materialized by codex/amata entrypoints into Crush config file | `home` | `home: ["<src>:.config/crush/crush.json:ro"]` |
| `CCR_CONFIG_JSON` | Materialized by codex/amata entrypoints into Claude Code Router config file | `home` | `home: ["<src>:.claude-code-router/config.json:ro"]` |
| `CODEX_PROMPT` (when file-backed) | Inline multiline prompt in env; shell/export-sensitive | `in` | `in: ["<src>:/in/codex-prompt.txt"]` + command uses `--prompt-file /in/codex-prompt.txt` |

Notes:

- There are no legacy special env keys that need migration to `out`.
- Non-special operational keys (for example `OPENAI_API_KEY`, `PLOY_GRADLE_BUILD_CACHE_URL`) remain in `envs`.
- Migration is deterministic and scriptable as key-based rewrites in persisted config entries and specs.

## Milestones

### Milestone 1: New contract types and validation

Scope:

- Add new spec fields and validators.
- Remove legacy fields from contracts.

Expected results:

- Specs containing legacy fields fail validation.
- Specs with new fields parse and validate deterministically.

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

- Generic resource materialization and mount plan execution.
- Remove tmp-bundle-only mount path and CA env preamble dependency.

Expected results:

- Containers receive correct `/in`, `/out`, `$HOME`, and CA materializations.
- Multiline env issues are eliminated for file-backed values.

Testable outcome:

- Node/workflow tests validate extraction safety, mount targets, mode enforcement, and `/out` upload continuity.

## Acceptance Criteria

- No accepted spec path uses `env_from_file`, `tmp_dir`, `tmp_bundle`, or `PLOY_CA_CERTS` env materialization.
- New fields validate with deterministic error messages for invalid destination/mode/domain.
- Stored spec entries validate canonical hash-first format (`shortHash:dst{:ro}` per field rules).
- Effective configuration precedence is exactly: server defaults < local config.yaml < spec.
- `in` mounts are read-only under `/in`; `out` mounts are read-write under `/out`; `home` mounts are under `$HOME`.
- `home` mode semantics are deterministic: default `rw`, optional `:ro`.
- CA is delivered via file materialization, not multiline env injection.
- Materialization logic is consolidated into shared compile/runtime pipelines with no parallel legacy paths.
- Existing `/out` final artifact upload behavior remains functional.
- Re-submitting identical file/folder content must reuse existing CAS objects and must not create additional object-store blobs.

## Risks

- Bulk rewrite of existing persisted env entries/specs to typed fields must be completed before rollout.

## Notes

- Hard cut migration impact is expected to be low for current active work. Existing in-flight migrations can be moved manually as part of rollout.
- `src:dst` and `src:dst:ro` notation intentionally follows familiar Docker bind-mount style. We do not expect practical adoption issues from delimiter semantics.
- Canonical `shortHash:dst{:ro}` format further reduces delimiter misuse risk because hash tokens do not contain `:`.

## References

- `cmd/ploy/mig_run_spec.go`
- `cmd/ploy/mig_run_spec_tmpbundle.go`
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
- `internal/nodeagent/execution_orchestrator_tmpbundle.go`
- `internal/store/schema.sql`
