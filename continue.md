# Session Continuation Notes

Date: 2025-10-30

- TLS over SSH tunnel (Option A) adopted.
  - Control-plane cert re-issued with IP SAN for 45.9.42.212; node client and CLI now verify strictly.
  - HTTP client sets SNI to descriptor address when base URL is loopback to keep TLS strict.
- Cluster join
  - 45.9.42.212 registered as a node; ployd restarted; job-claim loop active.
- Runtime
  - Docker present and healthy.
  - IPFS Cluster reachable on 9094; env wired via ployd.yaml.
  - SHIFT not installed (ok for now).
- Mods execution
  - Tickets submit and jobs claim; one job shows `succeeded` immediately with `stepworker: assignment missing step manifest`.
  - Root cause: plan stage lacked `step_manifest`.
- Next tasks
  1) Finalize manifest format (done): docs/next/manifest/README.md, examples, and docs/next/schema.json.
  2) Implement server-side builder for `plan` (inject `step_manifest` when missing).
  3) Swap example images to real planner/ORW images and arguments.
  4) Re-run `ploy mod run --follow` over tunnel and observe `running → succeeded` with artifacts.

Artifacts/Docs added in this session:
- docs/next/manifest/README.md — manifest concept and fields.
- docs/next/schema.json — JSON Schema (v1).
- docs/next/manifest/examples/{llm-plan.json,llm-exec.json,orw-apply.json}.
- docs/next/mod.md — linked to manifest docs.

Descriptor path: ~/.config/ploy/clusters (canonical).

---

# Continuation Plan (next session)

## Goal
Make Mods stages runnable without client manifests (start with `plan`), keep TLS strict over SSH tunnel (Option A).

## Next Steps

1) Server: Add plan manifest builder/translator
- internal/controlplane/mods/service.go
  - Add `buildPlanManifest(spec TicketSpec, def StageDefinition)`.
  - In `enqueueStage`, when `step_manifest` missing and stage is `plan` (or lane `mods-plan`):
    - Build `contracts.StepManifest` from repo URL + refs/commit; set runtime (image/command/env), outputs (plan.json), resources, retention.
    - Inject hydration hints: `hydration_repo_url`, `hydration_revision`, `hydration_input_name` = "workspace".
    - Validate and attach as JSON at `def.Metadata["step_manifest"]`.
  - Keep `prepareStageHydration` to swap in cached snapshot when available.

2) step_ref registry (optional)
- New: internal/controlplane/mods/steps/templates.go
  - Map `llm-plan`, `llm-exec`, `orw-apply` → default image/command/env/resources.
  - Apply template then overlay runtime overrides from manifest (if provided).

3) Tests
- internal/controlplane/mods/service_manifest_test.go
  - Submit ticket with `plan` and no manifest → metadata contains valid manifest + hydration hints.
- internal/controlplane/mods/service_hydration_test.go
  - Snapshot reuse rewrites manifest to BaseSnapshot CID.

4) Docs
- docs/next/manifest/README.md: note server may synthesize manifests from intent‑only submissions.
- docs/next/mod.md: reference per‑stage manifests and server synthesis.

5) Lab sanity (post‑builder)
- `./dist/ploy mod run --follow`
- Expect `plan` to run → `succeeded` with plan/data artifacts.

## File Pointers
- internal/controlplane/mods/service.go
- internal/controlplane/mods/steps/templates.go (new)
- internal/controlplane/mods/service_manifest_test.go (new)
