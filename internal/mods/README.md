# Mods Subsystem

The Mods subsystem orchestrates end-to-end code transformation and self-healing workflows. It runs planner → reducer → branch steps (human-step, llm-exec, orw-gen), applies diffs safely, validates builds, opens/updates merge requests, and learns from outcomes via a Knowledge Base.

## Features

- Workflow engine: planner → reducer → branches (human-step, llm-exec, orw-gen)
- Fanout healing: parallel branches with first-success-wins, bounded concurrency, timeouts
- Production job submission: HCL render/validate/submit via orchestration submitter; SeaweedFS artifact fetch
- Merge Requests: auth, templating, reporter; Git operations (clone/branch/commit/push)
- Diff apply + build gate: unified diff validation, path allowlist, staged commits, build check
- Gates: SBOM hooks and optional vulnerability gate; configurable timeouts and limits
- Knowledge Base: signatures, normalization, deduplication, compaction, locks (Consul/JetStream), metrics, maintenance, summary
- Events: controller reporter, MR events, structured event emission, log sanitization
- Defaults: image/model defaults, LLM tools/limits; MCP integration and env var generation
- Templates: planner/reducer/llm-exec/orw-apply HCL with variable substitution

## Files

- allocs_utils.go — Resolve first Nomad allocation ID via job-manager wrapper
- apply_and_build_adapter.go — Apply diff safely, commit, and run the build gate
- apply_build.go — Adapter to apply a diff then trigger build validation
- assets_llm_exec.go — Render llm-exec HCL template into workspace
- assets_orw_apply.go — Render ORW apply HCL template into workspace
- assets_planner.go — Write planner inputs.json and HCL template
- assets_reducer.go — Render reducer HCL template into workspace
- branch_chain.go — Branch chain model and execution helpers
- branch_chain_meta.go — Branch chain metadata helpers
- branch_chain_replayer.go — Replay just the HEAD step from a branch chain
- branch_step.go — Branch step struct and helpers
- build_gate.go — Build gate runner and response helpers
- build_guard.go — Guardrails around build submission and status
- cleanup.go — Cleanup helpers for temporary files/workspaces
- commit_push.go — Commit and push logic for repository changes
- config.go — YAML config (mods.yaml) types, loading, defaults, validation
- debug.go — Debug helpers and verbose logging toggles
- defaults.go — System defaults (timeouts, allowlists, plans)
- diff.go — Unified diff validation and application helpers
- enhanced_signatures.go — Enhanced KB signature extraction and comparisons
- events_emit.go — Event emission to controller and local sinks
- events_mr.go — Emit MR-related events and formatting
- events_util.go — Event reporting utilities (e.g., job submitted, alloc discovery)
- execution.go — Orchestration entrypoints for executing steps
- fanout_human_step.go — Human-step branch execution
- fanout_llm_exec.go — LLM-exec branch submission and result handling
- fanout_llm_helpers.go — Helpers for LLM-exec branch jobs
- fanout_orchestrator_core.go — Fanout engine core (parallel branches, first-success-wins)
- fanout_orw_apply.go — ORW generate/apply branch execution
- hcl_submitter.go — Interface/seam for HCL render/validate/submit
- healing_debug.go — Debug helpers for healing workflows
- healing_orchestration_adapter.go — Adapter wiring to orchestration submitter
- healing_orchestrator.go — Healing orchestrator composition
- human_step_helpers.go — Utilities for human-step branch (MR hints, notes)
- images.go — Image resolver for runner jobs (planner/reducer/LLM/ORW)
- infra.go — Infra resolver (controller URL, SeaweedFS URL, datacenter, registry)
- integrations.go — Production vs. test integrations factory wiring
- job_io.go — Read/write job artifacts (plan.json, next.json, diff.patch)
- job_mcp_env.go — Build MCP env vars and context for LLM-exec
- job_status_wait.go — Wait helpers for submitted jobs
- job_submit_planner.go — Submit planner job and fetch plan.json
- job_submit_reducer.go — Submit reducer job and fetch next.json
- job_submitter_types.go — Job submission type definitions and helpers
- job_template_subst.go — HCL template substitution for job env vars
- kb_compaction.go — KB storage compaction and maintenance
- kb_integration.go — KB recording and suggestion integration
- kb_locks.go — KB distributed locking interface and Consul implementation
- kb_locks_jetstream.go — KB distributed locking using JetStream KV CAS operations
- kb_maintenance.go — KB maintenance job triggers and utilities
- kb_metrics.go — KB metrics and counters
- kb_performance_analysis.go — KB performance/analysis helpers
- kb_signatures.go — Error signature extraction and canonicalization
- kb_storage.go — KB storage I/O (SeaweedFS paths and helpers)
- kb_summary.go — Produce KB summary/recommendations
- llm_defaults.go — LLM default model, tools, and limits
- llm_exec_helpers.go — LLM-exec helpers (paths, branch ID, outputs)
- log_sanitizer.go — Normalize and sanitize logs for KB and events
- mcp_integration.go — MCP config parsing, budget coercion, and context prefetch
- mocks.go — Test doubles for integrations (used in unit tests)
- modules.go — Small modular adapters (repo manager, build gate, MR manager)
- mr.go — Merge request operations (create/update) workflow
- mr_auth.go — MR auth helpers (token/env resolution)
- mr_template.go — MR title/body templating helpers
- orw_gen_helpers.go — Helpers for ORW generation branch
- orw_helpers.go — ORW execution helpers (paths, names)
- orw_prehcl.go — ORW HCL pre-processing helpers
- orw_submit.go — Submit ORW job and fetch diff.patch
- output.go — Output formatting helpers for CLI/API
- patch_normalization.go — Patch normalization and cleanup rules
- planner.go — CLI planner mode: render/submit planner and validate outputs
- push_events.go — Push event normalization and integration
- recipe_coords.go — Recipe coordinates (group/artifact/version) helpers
- recipe_subst.go — Recipe template substitution logic
- reducer.go — Reducer step orchestration
- remote.go — Remote repository URL handling and normalization
- repo_ops.go — Repository operations (clone/branch/commit/push)
- repo_ops_adapter.go — Adapter to Git provider/ops interfaces
- reporter.go — Controller/MR reporter abstraction
- run.go — Command entrypoints and runner setup
- runid.go — Stable run and branch identifiers
- runner.go — Main runner: orchestrates mods flow and healing
- runner_di.go — Runner dependency injection seams
- runner_helpers.go — Small helpers extracted from runner
- runner_results.go — Result aggregation helpers (winner, losers)
- schema.go — JSON/YAML schema shims and helpers
- self_healing.go — Healing entrypoints and decision logic
- signatures.go — Minimal signature helpers used outside KB
- step_types.go — Canonical step types and normalization
- steps_orw_apply.go — ORW-apply step execution helpers
- test_services.go — Integration helpers (test-only utilities used by tests)
- types.go — Job/plan/branch types and interfaces
- utilities.go — Generic utility helpers (paths, files, HTTP)
- vuln_gate.go — Vulnerability gate based on SBOM/NVD

## KB Locking Model

The Knowledge Base uses distributed locking to ensure data consistency during concurrent operations. Two backend implementations are supported:

### Consul KV Locking (Legacy)
- Uses Consul sessions for lock management with TTL-based expiry
- Lock acquisition creates a session and attempts to acquire the KV lock
- Lock release destroys the session, automatically releasing the lock
- Suitable for existing Consul-based deployments

### JetStream KV Locking (Default)
- Uses NATS JetStream KV Compare-And-Swap (CAS) operations for lock management
- Lock acquisition uses KV Create/Update with revision-based ownership verification
- Lock release uses KV Delete with revision check to ensure ownership
- Publishes lock events on `mods.kb.lock.acquired.<kb-id>`, `mods.kb.lock.released.<kb-id>`, and `mods.kb.lock.expired.<kb-id>`
- Enables immediate maintenance job triggering via event subscription and removes session heartbeats

### Configuration
JetStream is required for KB locking; the legacy `PLOY_USE_JETSTREAM_KV` Consul fallback has been removed. Locks live in the `mods_kb_locks` bucket under `writers/<kb-id>`.

### Lock Event Integration
When using JetStream, maintenance jobs can subscribe to lock release events for immediate triggering:
- `mods.kb.lock.released.java/signature` → triggers summary rebuild for that signature
- `mods.kb.lock.released.maintenance/*` → ignored (prevents recursive triggering)
- Other patterns → may trigger general maintenance based on configuration
