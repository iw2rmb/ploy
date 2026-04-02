# Global Env Targets and Materializers (`envs`)

## Summary

Replace scope-based global env behavior with a target-based model that supports one key across multiple runtime targets and keeps certificate handling as one special-case materializer (`PLOY_CA_CERTS`) on top of a generic env propagation core.

## Scope

In scope:
- Domain/store/API/CLI migration from `scope` to `target`.
- Multi-target storage and operations for the same key.
- Deterministic ambiguity behavior for key-only operations.
- Generic target-based propagation for server, nodes, gates, and steps.
- Shared materializer model for special keys, with `PLOY_CA_CERTS` as first built-in.
- Documentation and tests aligned with the new contract.

Out of scope:
- Backward compatibility for legacy `scope` records.
- Data migration for existing installations.
- New special key handlers beyond `PLOY_CA_CERTS`.
- Changes to unrelated run/job lifecycle semantics.

## Why This Is Needed

Current behavior mixes two models:
- Routing model uses `scope` values (`all`, `migs`, `heal`, `gate`).
- Runtime cert wiring already uses `PLOY_CA_CERTS` in multiple entrypoints.
- Gate/ORW and global env docs still rely on `CA_CERTS_PEM_BUNDLE`.

This causes inconsistent operator UX and repeated cert-specific logic across layers. We need one contract where env routing is generic and cert handling is an extension, not a special case in the routing core.

## Goals

- One key can be set once and applied to multiple targets.
- Core env propagation remains key-agnostic.
- CLI and API share identical ambiguity semantics.
- Precedence rules are deterministic and testable.
- Certificate handling is unified under `PLOY_CA_CERTS` with one materializer contract.

## Non-goals

- Preserve old `--scope` UX.
- Preserve `CA_CERTS_PEM_BUNDLE` compatibility.
- Implement compatibility aliases in server/store.
- Introduce speculative handlers for future env keys.

## Current Baseline (Observed)

- `GlobalEnvScope` drives parsing/matching in domain and server paths: `internal/domain/types/scope.go`, `cmd/ployd/server.go`.
- `config_env` persists one row per `key` with `scope` column and key primary key: `internal/store/schema.sql`, `internal/store/queries/config_env.sql`.
- HTTP handlers use key-level CRUD with `scope` in payloads: `internal/server/handlers/config_env.go`.
- CLI uses `--scope` and key-level set/show/unset semantics: `cmd/ploy/config_env_command.go`.
- Claim-time filtering is scope-driven: `internal/server/handlers/claim_spec_mutator_base.go`.
- Runtime deploy seeds `CA_CERTS_PEM_BUNDLE` from `PLOY_CA_CERTS`: `deploy/runtime/run.sh`.
- Server/node entrypoints consume `PLOY_CA_CERTS` path-style cert input: `deploy/images/server/entrypoint.sh`, `deploy/images/node/entrypoint.sh`.
- Gate/ORW scripts consume `CA_CERTS_PEM_BUNDLE` inline PEM style: `internal/workflow/step/gate_command.go`, `deploy/images/orw/orw-cli-gradle/orw-cli.sh`, `deploy/images/orw/orw-cli-maven/orw-cli.sh`.

## Target Contract or Target Architecture

### Model and boundaries

- Persisted model: `(key, target, value, secret, updated_at)`.
- Persisted targets: `server`, `nodes`, `gates`, `steps`.
- CLI-only aliases:
  - `jobs` expands to `gates,steps`.
  - `all` expands to `server,nodes,gates,steps`.
- No persisted alias values.

### API and CLI operation contract

- `PUT /v1/config/env/{key}` upserts one `key+target` entry.
- `GET /v1/config/env` lists all entries (deterministic order).
- `GET /v1/config/env/{key}`:
  - with `target`: return exact `key+target` entry.
  - without `target`: return entry only when exactly one target exists; otherwise return ambiguity error.
- `DELETE /v1/config/env/{key}` follows the same ambiguity rule as `GET`.
- CLI `show` and `unset` mirror the same rule via `--from`.

### Propagation rules

- Job routing:
  - `gates` target applies to `pre_gate`, `re_gate`, `post_gate`.
  - `steps` target applies to `mig`, `heal`.
- Nodes routing applies to node-side execution contexts.
- Server routing mutates process env on set/unset and bootstrap load.
- Precedence:
  - Per-run env overrides global env.
  - For global collisions, job-target value overrides nodes-target value.

### Materializer model

- Routing layer is key-agnostic.
- Materializers are opt-in handlers for specific keys.
- Default behavior for non-special keys is plain env passthrough.
- `PLOY_CA_CERTS` materializer accepts either:
  - inline PEM content, or
  - readable file path.
- Materializer output is uniform trust-store setup behavior across server/node/gate/step/ORW contexts.

### Backward-compatibility policy

- This design is a hard cut for test installations.
- No legacy row translation and no compatibility path for old scope-based data.

## Implementation Notes

- Domain/store:
  - replace `GlobalEnvScope` with `GlobalEnvTarget` types and parsers.
  - switch `config_env` SQL contracts and generated sqlc code to `key+target` CRUD.
- Server/API:
  - refactor holder and handlers to multi-target key operations.
  - standardize ambiguity error mapping.
  - update OpenAPI refs and schema names in `docs/api/OpenAPI.yaml` and path/schema files.
- CLI:
  - replace `--scope` with `--on` and `--from`.
  - implement selector expansion and strict ambiguity handling.
- Propagation:
  - replace scope filter logic in claim mutators with target routing.
  - keep merge precedence explicit and test-covered.
- Materializers:
  - introduce shared certificate handling path for gate command, ORW wrappers, and runtime scripts.
  - remove legacy `CA_CERTS_PEM_BUNDLE` key usage in runtime and tests.

## Milestones

### Milestone 1: Target model and persistence

Scope:
- Domain/store contracts and sqlc outputs for `key+target`.

Expected results:
- Store supports multiple target rows for one key with deterministic list/get order.

Testable outcome:
- `go test ./internal/domain/types ./internal/store ./cmd/ployd` passes.

### Milestone 2: HTTP contract and ambiguity semantics

Scope:
- Handler and OpenAPI migration from `scope` to `target`.

Expected results:
- API supports unambiguous key operations and explicit target selection for ambiguous keys.

Testable outcome:
- `go test ./internal/server/handlers -run ConfigEnv` and `go test ./docs/api -run OpenAPI` pass.

### Milestone 3: CLI selectors and parity with API

Scope:
- `--on` and `--from` UX and selector expansion.

Expected results:
- CLI behavior mirrors API ambiguity semantics and target rules.

Testable outcome:
- `go test ./cmd/ploy -run ConfigEnv` and `go test ./cmd/ploy -run HelpRegression` pass.

### Milestone 4: Generic propagation and precedence

Scope:
- Target-based claim merge and runtime consumption in server/node paths.

Expected results:
- Correct target-to-job routing and stable precedence across per-run/global collisions.

Testable outcome:
- `go test ./internal/server/handlers ./internal/nodeagent` passes with updated target routing tests.

### Milestone 5: Materializers and `PLOY_CA_CERTS`

Scope:
- Shared materializer model and cert handling migration.

Expected results:
- Certificate setup behavior is uniform across entrypoints, gate/step, and ORW wrappers.

Testable outcome:
- `go test ./internal/workflow/step ./internal/nodeagent ./internal/server/handlers` and shell syntax checks pass.

### Milestone 6: Docs convergence

Scope:
- Operator/API docs fully aligned to target model and materializer semantics.

Expected results:
- No scope-era or legacy cert key instructions remain.

Testable outcome:
- `go test ./docs/api` and `~/@iw2rmb/amata/scripts/check_docs_links.sh` pass.

## Acceptance Criteria

- All persisted global env data is target-based (`key+target`) with deterministic ordering.
- API and CLI both enforce the same ambiguity behavior for key-only operations.
- Claim/env merge behavior matches target mapping and precedence rules.
- `PLOY_CA_CERTS` is the only certificate key used in runtime contracts.
- Routing layer has no cert-specific logic; cert handling exists only in materializer paths.
- Tests and docs reflect shipped behavior.

## Risks

- Ambiguity rule changes may break ad-hoc scripts that assumed key-only delete/show.
- Missing one legacy key reference can leave partial cert behavior in one execution path.
- Cross-surface drift between CLI/API/docs is likely unless verified in one slice.
- Hard-cut semantics can silently clear operator expectations if test environments are reused.

## References

- `roadmap/envs.yaml`
- `internal/domain/types/scope.go`
- `internal/store/schema.sql`
- `internal/server/handlers/config_env.go`
- `internal/server/handlers/claim_spec_mutator_base.go`
- `cmd/ploy/config_env_command.go`
- `deploy/runtime/run.sh`
- `deploy/images/server/entrypoint.sh`
- `deploy/images/node/entrypoint.sh`
- `internal/workflow/step/gate_command.go`
- `deploy/images/orw/orw-cli-gradle/orw-cli.sh`
- `deploy/images/orw/orw-cli-maven/orw-cli.sh`
- `docs/envs/README.md`
