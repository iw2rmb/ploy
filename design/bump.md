# Goal-Bounded Bump and Re-Gate Recovery

## Summary
Define a deterministic dependency bump algorithm for migration runs where impacted libraries are unknown upfront.

The model is:
- no runtime hooks,
- no SBOM before gates,
- healing only from `post_gate`/`re_gate`,
- bump cycles executed inside `mig` and `heal` jobs.

## Scope
In scope:
- Ploy orchestration changes for migration bump cycles.
- `mig` and `heal` objective execution model.
- Re-gate-driven validation loop and artifact contract.

Out of scope:
- Amata executor internals (see `~/@iw2rmb/amata/design/short-pooling-download.md`).
- Concrete ploy-lib prompts/scripts (see `~/@scale/ploy-lib/design/re_gate.md`).

## Why This Is Needed
Mass migrations cannot rely on fixed dependency allowlists. Bump actions must expand from observed build failures while preserving deterministic stop rules.

## Goals
- Bound all bump edits to migration objective and observed evidence.
- Keep recovery points at every successful gate.
- Support typed OpenRewrite where signatures must be migrated through bridge versions.

## Non-goals
- Restoring legacy hook runtime behavior.
- Introducing separate top-level bump job types.

## Current Baseline (Observed)
- Initial chain currently inserts SBOM before pre/post gates: `internal/server/handlers/migs_ticket.go`.
- Runtime hook chain is currently inserted after SBOM success: `internal/server/handlers/jobs_complete_service_runtime_hooks.go`.
- Heal is currently considered for any failed gate (`pre|post|re`): `internal/workflow/lifecycle/orchestrator.go`.
- Heal insertion currently rewires `failed_gate -> heal -> retry_sbom -> re_gate`: `internal/server/handlers/nodes_complete_healing.go`.

## Target Contract or Target Architecture
### Objective model
- `objective_id` is migration identity (`mig` name).
- Execution lanes inside the same objective:
  - `mig` lane (planned migration actions),
  - `heal` lane (child cycles created at runtime).

### Job flow
Per repo run flow:
1. `pre_gate` (must pass; no heal on failure).
2. `mig` (ORW recipe + LLM migration steps).
3. `post_gate`.
4. On `post_gate` failure: start `heal -> re_gate` loop.
5. Repeat `heal -> re_gate` until success or retries exhausted.
6. Persist final SBOM at end of successful flow.

### Healing trigger rules
- Only `post_gate` and `re_gate` failure can trigger heal.
- `pre_gate` failure is terminal for repo attempt.

### Dependency bump loop inside heal/mig
1. Apply candidate bump edit(s).
2. Call build (`re_gate`) and capture output.
3. If success, keep bump and continue objective flow.
4. If failure:
   - classify (`deps`, `code`, `infra`),
   - for `deps` choose downgrade/bridge/additional package only from current evidence,
   - for signature drift use bridge path:
     1. lower to overlap/deprecated version,
     2. run ORW migration,
     3. bump to target.
5. Repeat until success or budget exhausted.

### Re-gate artifacts
- Internal build calls in same workspace produce numbered artifacts in `/out`, e.g.:
  - `re_build-gate-1.log`
  - `re_build-errors-1.yaml`
  - `re_build-gate-2.log`
- Heal reads these local artifacts directly (no mandatory download for this flow).

## Implementation Notes
- Remove hook planning/insertion from migration lifecycle paths.
- Remove SBOM-prelude jobs from pre/post gate chain; keep final SBOM persistence only.
- Change completion routing so only `post_gate`/`re_gate` fail can evaluate healing insertion.
- Update healing insertion to drop retry-SBOM dependency in the loop.
- Keep classpath contract for typed ORW by sourcing from latest successful gate artifact (SBOM fallback allowed only when present).
- Keep compatibility with future generic amata polling/download contract in `~/@iw2rmb/amata/design/short-pooling-download.md`.

## Milestones
1. Lifecycle simplification.
- Scope: remove hook and pre/post SBOM-prelude flow wiring.
- Testable outcome: planned chains contain `pre_gate -> mig -> post_gate` base.

2. Healing trigger narrowing.
- Scope: heal only from failed `post_gate`/`re_gate`.
- Testable outcome: failed `pre_gate` cancels repo flow without heal.

3. Re-gate loop and artifact contract.
- Scope: numbered re-build artifacts, retry logic.
- Testable outcome: heal loop consumes `/out/re_build-*` deterministically.

4. Typed ORW continuity.
- Scope: classpath for `mig`/`heal` from successful gate lineage.
- Testable outcome: ORW-based heal works without SBOM-prelude jobs.

## Acceptance Criteria
- No runtime hooks are scheduled/executed in migration flow.
- Base chain runs without SBOM-prelude jobs.
- Only `post_gate`/`re_gate` failures can create heal children.
- Final successful flows persist final SBOM.
- Bump attempts are bounded and auditable from artifacts + metadata.

## Risks
- Removing SBOM-prelude weakens early dependency visibility unless gate artifacts are complete.
- ORW typed attribution may fail when classpath capture from gate is incomplete.
- First rollout may surface hidden dependencies formerly handled by hooks.

## References
- `internal/server/handlers/migs_ticket.go`
- `internal/server/handlers/jobs_complete_service_runtime_hooks.go`
- `internal/workflow/lifecycle/orchestrator.go`
- `internal/server/handlers/nodes_complete_healing.go`
- `~/@iw2rmb/amata/design/short-pooling-download.md`
- `~/@scale/ploy-lib/design/re_gate.md`
