# SBOM/Hooks Incident Analysis (2026-04-08)

## Scope
This analysis covers failures observed on April 8, 2026 in `roadmap/sbom-hooks` behavior: zero-time SBOM jobs, hook planning/execution mismatches, non-deterministic failure handling in earlier builds, and missing spec bundle blobs.

## Executive Summary
The rollout shipped contract and scheduling surface changes without end-to-end runtime implementation parity. The system exposed `sbom`/`hook` jobs as if fully implemented, but node execution paths were still placeholder/stub logic. At the same time, planning semantics created hook jobs unconditionally, and delivery governance allowed roadmap phases to be marked done despite recorded gaps. This produced user-visible false positives: jobs looked present and successful while core behavior was absent.

## Proven Facts
1. SBOM execution is stubbed in nodeagent runtime.
- `internal/nodeagent/execution_orchestrator_jobs.go:166` (`executeSBOMJob`) writes a canonical file and reports success.
- `internal/nodeagent/execution_orchestrator_jobs.go:516` (`writeCanonicalSBOMOutput`) is deterministic placeholder output.
- This explains observed `sbom` durations around `0.0s` and empty/unchanged SBOM index outcomes.

2. Hook execution path is not real step execution.
- `internal/nodeagent/execution_orchestrator_jobs.go:204` (`executeHookJob`) copies snapshot files (`/in`/`/out`) and reports success.
- There is no hook-step container command execution in this path.
- This explains `hook` durations around `0.1s` with no meaningful hook side effects.

3. Hook jobs are planned into the chain before runtime condition evaluation.
- `internal/server/handlers/migs_ticket.go:247-255` appends hook drafts for each resolved hook source in each cycle prelude.
- Planning occurs during `createJobsFromSpec` before matcher decisions.
- Runtime may later mark job as skip (`HookShouldRun=false`), but job already exists and is visible as planned.

4. Missing spec bundle blob caused terminal hook claim failure in observed run.
- `internal/server/handlers/nodes_claim_hook_once.go:318` returns error when object storage blob is missing.
- Observed error: `spec bundle blob "spec_bundles/dwjioipM/bundle.tar" is missing from object storage`.
- This is a real storage/consistency failure, not a matcher/schema failure.

5. Roadmap state signaling drift exists.
- `roadmap/bump.yaml` is the active roadmap artifact for this scope.
- `roadmap/sbom-hooks/phase-*.yaml` are marked `done: true` while containing `reviews.gaps` entries documenting unmet behavior.
- Delivery status was therefore not trustworthy as a readiness signal.

## Root Causes
1. Implementation/contract split without runtime parity gates.
- Job types, schema, and scheduling were implemented first, but runtime executors remained placeholders.
- No hard gate prevented shipping when runtime behavior did not match declared contract.

2. Incorrect planning boundary for hook conditions.
- Conditions were treated as runtime skip metadata, not as planning-time inclusion criteria.
- Result: hooks appear in plan regardless of condition truth.

3. Weak release governance around roadmap completion.
- Phases were marked `done` while review artifacts already documented critical gaps.
- No automated policy blocked completion state on unresolved runtime gaps.

4. Blob persistence reliability gap for spec bundles.
- Runtime depends on DB metadata + object storage blob existence.
- There is no mandatory pre-run integrity check for `bundle_map` references to ensure blobs still exist.
- Failure therefore appears late at claim time.

## Contributing Factors
1. Tests focused on unit-level behavior and scheduling shape rather than full run observables.
- Missing mandatory e2e assertions for: non-zero SBOM work, hook container invocation, and condition-based plan inclusion.

2. Misleading success semantics.
- Stub jobs reported success with minimal duration, masking missing functionality.

3. Operational observability gaps.
- No explicit health signal for “DB row exists but blob missing” for spec bundles before claim.

## Why This Escaped
1. Completion criteria were artifact-based (code merged, tests green) instead of behavior-based (real run evidence).
2. “Done” marking lacked enforcement against known `reviews.gaps`.
3. Job UX showed planned/executed states without distinguishing placeholder execution from functional execution.

## Immediate Containment Already Applied
1. Hook spec parity and deterministic terminal claim failure improvements were applied in commit `a5bad834`.
- Added hook step field parity (`ca/in/out/home`) acceptance.
- Claim-time deterministic hook spec errors now fail job/run instead of looping claim retries.

## Required Remediation Direction
1. Replace SBOM/hook stubs with real execution paths and artifact ingestion.
2. Change scheduler so hook jobs are created only when conditions evaluate true for that cycle.
3. Add storage integrity checks for all bundle-map referenced blobs before queueing hook jobs.
4. Make roadmap completion non-overridable when runtime acceptance checks fail.

## Confidence and Unknowns
- High confidence: causes tied to stub runtime and unconditional planning are proven by code paths above.
- Medium confidence: exact mechanism that removed blob `spec_bundles/dwjioipM/bundle.tar` (external object-store lifecycle, manual deletion, or failed write consistency) is not yet proven from current evidence and needs dedicated storage audit logs.
