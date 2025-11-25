# Checkpoint — Mods E2E (ORW) Current State (Nov 5, 2025)

## Summary
We’ve unblocked the control plane and workers, implemented proper container retention and log streaming, and verified the container runtime path with a self‑test. The OpenRewrite (ORW) step still fails quickly with exit code 1 and no mod logs exposed to the stream, so we’re adding a targeted retained probe on the ORW image to capture first‑mile failure details.

Update this document as you go.

## What’s Fixed
- Container lifecycle: disabled Docker AutoRemove and added explicit delete after logs are fetched; added `--retain-container` to keep containers for inspection.
- SSE logs: node uploads gzipped chunks; server gunzips and publishes per‑line SSE; log ingest routes now publish to SSE.
- Diagnostics in status: `GET /v1/mods/{id}` now includes `metadata.node_id` (which worker claimed the run) and `metadata.reason` (e.g., `exit code 1`).
- Workers re‑added: re‑provisioned worker‑b (193.242.109.13) and worker‑c (45.130.213.91) with correct mTLS config; claims work again.
- Self‑test: a retained run with `alpine:3.20` and a simple command succeeds on worker‑c, validating Docker and claims path.

## What’s Observed (but not yet explained)
- ORW runs are consistently claimed (often by worker‑b) and terminate in ~19s with `reason = "exit code 1"`.
- No mod logs are visible for these failing runs via SSE, and the `/out` bundle is empty; retained container matching the ticket label is typically absent, suggesting failure occurred before container start or the container exited without producing stdout/stderr content that we capture.
- Earlier on worker‑b we saw historical “Cannot connect to the Docker daemon” errors; those predate re‑bootstrap and do not appear in the latest claims.

## Not Yet Confirmed
- Whether the ORW container actually starts on worker‑b after the recent bootstrap, and whether the failure is due to:
  - Maven/OpenRewrite plugin resolution (egress/DNS/Maven Central access),
  - Workspace hydration mismatch (no `pom.xml` under `/workspace`), or
  - Command/entrypoint mismatch (we removed placeholder commands; the image CMD should run `mods-orw --apply`).
- That the retained flag is honored on the exact failing step (we propagate `retain_container`, and we label containers; absence suggests pre‑container failure or another early exit path).

## Next Actions
1) Run a retained ORW‑image probe with a verbose Maven command to capture first‑mile failure (env, `mvn -v`, list `/workspace`, plugin run `-X`).
   - The run is submitted with `--mod-image mods-openrewrite:latest`, an explicit `--mod-command` wrapped in `/bin/sh -lc`, and `--retain-container`.
   - Deliverables: TICKET, claiming `node_id` + `ip`, retained container ID (via label `com.ploy.run_id=<TICKET>`), and first 200 lines of `docker logs`.
2) If the probe shows Maven egress failures, set or validate node‑level Docker/systemd proxy (and test `wget` in the container).
3) If the probe shows missing `/workspace/pom.xml`, inspect hydration on the node (ensure repo/refs materialize to `/workspace`).
4) If logs remain empty but the container starts, add early echo/tee in the ORW entrypoint to guarantee an emitted line before Maven starts, and re‑publish image.
5) After root cause is confirmed on worker‑b, re‑run `tests/e2e/mods/scenario-orw-pass.sh` with `--retain-container` to confirm logs and artifacts are emitted and downloadable.

## Ground Truth (as of this checkpoint)
- Control plane: `https://45.9.42.212:8443` (mTLS). Ticket status returns mods‑style summary with `node_id`/`reason`.
- Workers:
  - worker‑b (193.242.109.13): claims ORW; recent runs end `exit code 1`; Docker reachable interactively.
  - worker‑c (45.130.213.91): self‑test succeeded; Docker reachable.
- Logging: SSE pipeline is enabled end‑to‑end; absence of logs on ORW indicates the container likely isn’t producing (or step fails before start), not a transport issue.

## Notes
- All retention and log‑fanout changes are unit‑tested; control plane and node binaries have been rebuilt and deployed.
- Containers created by steps carry `com.ploy.run_id=<TICKET>` (run/ticket UUID) for discovery; removal is explicit and skipped when retained. The historic mislabeling with a step identifier has been fixed.
- Type semantics are hardened in code (e.g., `RepoURL`, `GitRef`, `CommitSHA`, `TicketID/RunID`, `StageID`, resource units). JSON over the wire remains unchanged (strings with the same shapes), preserving API compatibility.

---

## GitLab MR — Discovery (Phase A / Step 1)

What I mined from repo history (prior MR paths):

- Provider and HTTP shapes (commit e087bc49, 2025‑09‑05)
  - File: `internal/git/provider/gitlab.go` (removed in da348c89).
  - Endpoints used:
    - GET `/_api/v4/projects/{project}/merge_requests?source_branch={branch}&state=opened` (check existing MR).
    - POST `/_api/v4/projects/{project}/merge_requests` body `{title, description, source_branch, target_branch, labels}` (create MR).
    - PUT  `/_api/v4/projects/{project}/merge_requests/{iid}` body `{title, description, target_branch, labels}` (update MR).
  - Auth: `Authorization: Bearer <token>`; base URL from `GITLAB_URL` (default `https://gitlab.com`); token from `GITLAB_TOKEN`.
  - Project path extraction: parsed repo HTTPS URL; trimmed leading `/` and `.git`.

- Runner integration (same commit series)
  - File: `internal/cli/transflow/runner.go` — after push, called `CreateOrUpdateMR`.
  - MR title: `"Transflow: <workflow ID>"`.
  - MR labels: `ploy, tfl` (CSV in request payload).
  - MR description: rendered from template in `internal/mods/mr_template.go` (also existed under `internal/cli/transflow/mr_template.go`).
  - Branch name template: `workflow/<id>/<timestamp>`.

- Per‑run auth mapping (commit 48f4500f, 2025‑09‑20)
  - File: `internal/mods/mr_auth.go` — mapped mod config `mr.token_env` to process env; set `PLOY_GITLAB_PAT` and `GITLAB_URL` for provider/git ops.

- Control‑plane signer (removed)
  - Files: `internal/config/gitlab/*`, `internal/api/httpserver/controlplane_signer.go`, docs under `docs/design/gitlab-*`.
  - Provided AES‑key‑backed signer/rotation/revocation; deleted in pivot (docs/envs: "GitLab Signer (Removed)").

Implications for current plan:
- Request shapes and field names match GitLab v4; safe to reuse.
- Title/description templates exist; can port minimal versions (rename Transflow → Ploy Mods).
- Env names diverged historically (`GITLAB_TOKEN` vs `PLOY_GITLAB_PAT`). Plan should standardize on server‑stored PAT with per‑run override flag and map to provider without leaking to logs.

## Updates — 2025‑11‑04

What I attempted (literal to Next Actions):
- Built CLI (`dist/ploy`).
- Pointed CLI to control plane: `PLOY_CONTROL_PLANE_URL=https://45.9.42.212:8443` (mTLS via default descriptor).
- Submitted retained ORW probe twice using `docker.io/iwtormb/mods-openrewrite:latest`, with `--mod-command` printing env/versions, listing `/workspace`, and invoking the Rewrite plugin (`-X`).
  - Tickets: `fbd8ea42-e9b4-4f2f-bb22-34d573729de3` and `3ccec0b9-a83c-4fcb-b310-a99b6a1b6ccc`.

Observed blockers (initial):
- Both workers reported drained=true via `GET /v1/nodes`.
  - worker‑c 45.130.213.91 (drained=true)
  - worker‑b 193.242.109.13 (drained=true)
- Result: new tickets remained `pending/queued`; no node claims, no SSE logs.

Actions taken after approval:
- Undrained worker‑b via API; then attempted `ploy rollout nodes --selector worker-b` to refresh `ployd-node`.
- Rollout failed at the heartbeat confirmation step, but worker‑b resumed heartbeating (hb updated to ~23:24Z).
- Undrained both worker‑b and worker‑c; current state:
  - worker‑b 193.242.109.13 — drained=false, last_heartbeat recent (23:24Z)
  - worker‑c 45.130.213.91 — drained=false, last_heartbeat stale (22:28Z)

Current status:
- Despite worker‑b being undrained and heartbeating, the submitted tickets (including fresh `22886220-332d-46ca-8215-2368e6e74f3f`) remain `pending` with stage `queued`; no `node_id` assigned yet and no SSE logs.

Hypotheses:
- Claim loop not running or failing on worker‑b (mTLS OK for heartbeat, but claim POST might be failing auth/role).
- Server returns 204 on claim due to a queue filter mismatch (unlikely; runs are `queued` and nodes undrained).
- Transient rollout left service updated but not fully restarted; needs a clean restart.

Next steps proposed:
- SSH to worker‑b and check ployd‑node logs; verify `/v1/nodes/{id}/claim` attempts and responses; restart service if needed.
- If logs show claim auth errors, verify node cert OU (`worker`) and server authorizer; re‑issue node certs if required.
- Once claims activate, re‑run the retained ORW probe and capture SSE + retained container evidence.

## Result — 2025‑11‑05 (Post‑fix state)

Fix applied:
- Worker‑b claim/heartbeat 404s were caused by a NodeID mismatch.
  - `/etc/ploy/ployd-node.yaml` had `node_id: 57ffe804-a72d-47ab-b7af-14a5a4605a49`.
  - Control plane reports worker‑b id: `28587647-682f-4ab1-b5a4-a2d036a35a20`.
  - Updated the node config to the correct id and restarted `ployd-node`.
  - After restart, worker‑b immediately claimed pending runs.

Probe (retained) — details:
- Ticket: `35ecc92e-c305-400e-8b05-291f03923530`
- node_id: `28587647-682f-4ab1-b5a4-a2d036a35a20` (worker‑b 193.242.109.13)
- Image: `docker.io/iwtormb/mods-openrewrite:latest`
- Command (argv): `["--apply","--dir","/workspace","--out","/out"]`
- Retained container: `fe3caaee478d`
- Exit: `1` (reason in ticket metadata: `exit code 1`)

First 200 lines of docker logs (probe):

```
[mod-orw] Running OpenRewrite recipe: org.openrewrite.java.migrate.UpgradeToJava17
[mod-orw] Coordinates: org.openrewrite.recipe:rewrite-java-17:2.6.0 (plugin 6.18.0)
Apache Maven 3.9.11 (3e54c93a704957b63ee3494413a2b544fd3a825b)
Maven home: /usr/share/maven
Java version: 17.0.16, vendor: Eclipse Adoptium, runtime: /opt/java/openjdk
Default locale: en_US, platform encoding: UTF-8
OS name: "linux", version: "6.8.0-86-generic", arch: "amd64", family: "unix"
[DEBUG] Created new class realm maven.api
[DEBUG] Importing foreign packages into class realm maven.api
[DEBUG]   Imported: javax.annotation.* < plexus.core
[DEBUG]   Imported: javax.annotation.security.* < plexus.core
[DEBUG]   Imported: javax.inject.* < plexus.core
[DEBUG]   Imported: org.apache.maven.* < plexus.core
[DEBUG]   Imported: org.apache.maven.artifact < plexus.core
...
```

Tail excerpt (failure cause):

```
Caused by: org.eclipse.aether.transfer.ArtifactNotFoundException: Could not find artifact org.openrewrite.recipe:rewrite-java-17:jar:2.6.0 in central (https://repo.maven.apache.org/maven2)
```

Interpretation:
- Claims/logs path is healthy (container started; logs retained; Maven executed).
- Not an egress/DNS block; Maven reached Central but the requested recipe artifact/version was not found.

Follow‑up runs and confirmations:
- Using the scenario coords from tests (env only):
  - `RECIPE_GROUP=org.openrewrite.recipe`
  - `RECIPE_ARTIFACT=rewrite-migrate-java`
  - `RECIPE_VERSION=3.20.0`
  - `RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17`
- Ticket `1e7b3986-756c-4a44-9cac-3a76e07706ec` (main→main) — Succeeded on worker‑b; retained container exited 0; ORW logs show successful run.
- Ticket `5d89be9c-9462-4681-a2b9-e305eeec7fca` (e2e/fail-missing-symbol→main) — Succeeded on worker‑b; confirms hydration and apply succeed on failing baseline.
- Diffs are stored in DB (table `diffs`). New endpoints added (requires server rollout):
  - `GET /v1/mods/{id}/diffs` — list per‑run diffs; `GET /v1/diffs/{id}?download=true` — download gzipped patch.
  - CLI: `ploy mod diffs <ticket> [--download] [--output <file>]`.

Decision — Build Gate metrics collection:
- Chosen approach: in‑process collection in node agent (Docker stats/inspect) rather than cAdvisor.
- Rationale: per‑run, low‑overhead, tight correlation to `run_id`, no extra daemon.
- If we expand to fleet/continuous metrics later, revisit cAdvisor/Prometheus.

Current gaps vs test goals (with latest collector changes):
1) Build Gate confirmation — We execute the Java build gate (`mvn test`) inside the node runner (Docker). Timings are persisted in run `stats` (`build_gate_duration_ms`), but `GET /v1/mods/{id}` does not expose `stats` yet.
   - Implemented on node side: collect build logs, pass/fail, duration, resource limits and usage; upload `build-gate.log` as artifact (≤256 KiB) tied to `run_id`/`stage_id`; record all metrics under `runs.stats.gate` (limits+usage+duration+passed).
   - Remaining: expose `runs.stats` (or just `runs.stats.gate`) via `GET /v1/mods/{id}` or a new `GET /v1/mods/{id}/stats`; print in CLI.
2) GitLab MR — Branch push + MR creation wired
   - Implemented path: global PAT/domain via control plane config with optional per‑run overrides; node pushes branch using a non‑persistent HTTP Authorization header and creates the MR via GitLab API with bounded retries; MR URL is attached under `runs.stats.metadata.mr_url` and surfaced by `GET /v1/mods/{id}` so `ploy mod inspect` prints it.

## Ticket Model for Multi-step Mods Runs — Implemented (2025-11-25)

**Goal**: Clarify control plane ticket model to treat runs as ordered sequences of steps for multi-step Mods runs.

**Changes**:
1. **API Types** (internal/mods/api/types.go):
   - Added `StepIndex` field to `StageStatus` (0-based position in run's step sequence).
   - Added `StageMetadata` struct with `step_index`, `step_total`, and `mod_image` fields.
   - Metadata is stored in `stages.meta` JSONB column, enabling ordered execution.

2. **Handler Logic** (internal/server/handlers/handlers_mods_ticket.go):
   - `submitTicketHandler` now parses spec and creates one stage per mod step.
   - For multi-step runs (`mods[]` array), creates stages named `mods-openrewrite-0`, `mods-openrewrite-1`, etc.
   - For single-step runs (`mod` or legacy top-level), creates one stage `mods-openrewrite`.
   - Each stage's `meta` JSONB includes `step_index`, `step_total`, and `mod_image` for diagnostics.
   - `getTicketStatusHandler` parses step metadata from `stages.meta` and exposes `step_index` in response.

3. **Tests** (internal/server/handlers/handlers_mods_ticket_test.go):
   - Added `TestSubmitTicketHandlerMultiStepCreatesMultipleStages` verifying 3 stages for 3 mods.
   - Added `TestSubmitTicketHandlerSingleStepCreatesOneStage` verifying backward compatibility.
   - Added `TestGetTicketStatusHandlerExposesStepIndex` verifying step metadata exposure.
   - All existing tests pass; backward compatibility maintained.

**Result**: Control plane now treats multi-step runs as ordered sequences of stages with explicit step indexing, enabling future rehydration with diffs from prior steps (ROADMAP.md line 24+).

## Multi-Node Mods Architecture and Rehydration Model — Documentation (2025-11-25)

**Context**: Implementation of ROADMAP.md line 73 — Document the multi-node Mods architecture and rehydration model with base clone + diff chain semantics and scheduler behaviour.

### Architecture Overview

The multi-node Mods architecture enables distributed execution of multi-step Mods runs across a cluster. Each step of a run can execute on a different node by reconstructing workspace state from an immutable base clone and an ordered chain of per-step diffs, eliminating the need for long-lived mutable workspaces.

### Core Components

**1. Base Clone + Diff Chain Model**

The architecture separates repository state into two layers:

- **Base Clone (Immutable Snapshot)**: A shallow git clone (`git clone --depth 1`) of the repository at `base_ref` (or pinned to `commit_sha` if provided). Created once per run, cached under `PLOYD_CACHE_HOME`, and never modified during execution.

- **Diff Chain (Ordered Modifications)**: A sequence of gzipped unified diffs captured after each step (gate + mod pair), stored in PostgreSQL with `step_index` metadata. Each diff represents the workspace changes produced by one step.

**Workspace Rehydration Formula**:
```
workspace[step_k] = copy(base_clone) + apply(diff[0]) + apply(diff[1]) + ... + apply(diff[k-1])
```

This ensures every node can independently reconstruct the exact workspace state needed for any step without requiring shared storage or long-lived workspaces.

**2. Step-Level Scheduler**

The scheduler treats multi-step runs as ordered sequences of claims. Step claiming enforces sequential consistency:

- **Step 0**: Claimable immediately (no dependencies)
- **Step k>0**: Claimable only after step k-1 succeeds

The `run_steps` table tracks per-step status (`queued → assigned → running → succeeded/failed`) and uses `FOR UPDATE SKIP LOCKED` to enable lock-free, concurrent claims across multiple nodes.

**3. Node Agent Execution Flow**

When a node claims a step:

1. **Base Clone Creation** (once per run):
   - Fetch via `git clone --depth 1 --branch base_ref`
   - Optionally pin to `commit_sha` via `git fetch origin commit_sha --depth 1 && git checkout FETCH_HEAD`
   - Cache under `PLOYD_CACHE_HOME/git-clones/{hash(repo_url, base_ref, commit_sha)}`

2. **Workspace Rehydration** (per step):
   - Copy base clone to ephemeral workspace
   - Fetch all diffs where `step_index < current_step` from control plane
   - Apply diffs sequentially via `git apply` to replay prior step changes

3. **Step Execution**:
   - Run build gate (if configured)
   - Execute mod container with rehydrated workspace
   - Capture gate/mod logs and resource metrics

4. **Diff Capture and Upload**:
   - Generate unified diff: `git diff HEAD` (relative to base clone)
   - Compress with gzip
   - Upload to control plane tagged with `step_index`, `run_id`, `stage_id`
   - Store in `diffs` table with `step_index` for ordering

5. **Cleanup**:
   - Remove ephemeral workspace after diff upload (except final step)
   - Retain base clone in cache for subsequent steps on same node

### Key Properties

**Correctness**:
- Sequential step claiming enforces dependencies: step k cannot start until k-1 completes successfully
- Diffs are ordered by `step_index` (0-based, sequential) and applied deterministically
- Rehydration produces identical workspace state regardless of which node executes the step

**Efficiency**:
- Shallow clones minimize network transfer (single-commit depth)
- Base clone caching eliminates redundant fetches for multi-step runs on same node
- Ephemeral workspaces enable parallel execution without workspace contention

**Flexibility**:
- Steps can execute on different nodes (multi-node scheduling)
- Steps can execute on the same node (single-node execution)
- Nodes dynamically claim steps based on availability and drain status

### Data Structures

**1. `run_steps` Table (Step Status Tracking)**:
```sql
CREATE TABLE run_steps (
  id           UUID PRIMARY KEY,
  run_id       UUID NOT NULL REFERENCES runs(id),
  step_index   INT NOT NULL,
  status       TEXT NOT NULL, -- queued | assigned | running | succeeded | failed | canceled
  node_id      UUID REFERENCES nodes(id),
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ,
  UNIQUE (run_id, step_index)
);
-- Index for claim queries: find oldest queued step with dependencies satisfied
CREATE INDEX run_steps_claim_idx ON run_steps (status, run_id, step_index);
```

**2. `diffs` Table (Diff Storage)**:
```sql
CREATE TABLE diffs (
  id          UUID PRIMARY KEY,
  run_id      UUID NOT NULL REFERENCES runs(id),
  stage_id    UUID NOT NULL REFERENCES stages(id),
  step_index  INT,  -- nullable for backward compatibility
  patch       BYTEA NOT NULL,  -- gzipped unified diff
  summary     JSONB NOT NULL,  -- {step_index, exit_code, timings}
  created_at  TIMESTAMPTZ DEFAULT now()
);
-- Index for rehydration queries: fetch diffs for step k (all j where 0 <= j < k)
CREATE INDEX diffs_run_step_idx ON diffs (run_id, step_index NULLS LAST);
```

**3. `RunOptions.Steps[]` (Multi-Step Spec)**:
```go
type RunOptions struct {
  BuildGate      BuildGateOptions      // Global gate config (shared across steps)
  Healing        *HealingConfig        // Gate failure recovery (shared)
  Steps          []StepMod             // Multi-step mods[] array
  // ... other fields
}

type StepMod struct {
  Image           string            // Mod container image (e.g., "mods-openrewrite:latest")
  Command         ExecutionCommand  // Command override (optional)
  Env             map[string]string // Step-specific environment variables
  RetainContainer bool              // Debug flag: keep container after execution
}
```

### Scheduler Behavior

**Claim Priority (nodes_claim.go)**:

The control plane handler prioritizes step-level claims before whole-run claims:

1. **Try step claim**: `ClaimRunStep(node_id)` — Returns oldest queued step where dependencies are satisfied
2. **Fallback to run claim**: `ClaimRun(node_id)` — Returns oldest queued single-step run (legacy path)
3. **No work**: Return 204 No Content

**Claim Query Logic (run_steps.sql:ClaimRunStep)**:

```sql
WITH eligible_steps AS (
  SELECT rs.id, rs.run_id, rs.step_index
  FROM run_steps rs
  INNER JOIN runs r ON r.id = rs.run_id
  INNER JOIN nodes n ON n.id = $1
  WHERE rs.status = 'queued'
    AND n.drained = false
    -- Step 0 can always be claimed; step k>0 requires step k-1 succeeded
    AND (
      rs.step_index = 0
      OR EXISTS (
        SELECT 1 FROM run_steps prev
        WHERE prev.run_id = rs.run_id
          AND prev.step_index = rs.step_index - 1
          AND prev.status = 'succeeded'
      )
    )
  ORDER BY r.created_at, rs.step_index  -- FIFO, then step order
  FOR UPDATE OF rs SKIP LOCKED          -- Lock-free concurrency
  LIMIT 1
)
UPDATE run_steps
SET status = 'assigned', node_id = $1, started_at = now()
FROM eligible_steps e
WHERE run_steps.id = e.id
RETURNING *;
```

**Claim Response (StartRunRequest)**:

When a step is claimed, the response includes `step_index` to constrain execution:

```json
{
  "id": "run-uuid",
  "repo_url": "https://github.com/example/repo.git",
  "base_ref": "main",
  "target_ref": "feature-branch",
  "commit_sha": "abc123def456",
  "step_index": 2,           // Present for multi-step claims
  "spec": {
    "mods": [
      {"image": "mods-plan:latest"},
      {"image": "mods-openrewrite:latest"},
      {"image": "mods-codex:latest"}  // This step (index=2) will execute
    ],
    "build_gate": "mvn test",
    "build_gate_healing": {"enabled": true, "max_attempts": 3}
  }
}
```

The node agent uses `step_index` to execute **only** step 2, rehydrating the workspace from base + diffs[0..1].

### Execution Orchestrator Loop (execution_orchestrator.go)

The orchestrator implements a step loop that rehydrates and executes each step in sequence:

```go
// For multi-node claims: startStepIndex = claimedStepIndex, stepCount = claimedStepIndex + 1
// For single-node execution: startStepIndex = 0, stepCount = len(Steps)

for stepIndex := startStepIndex; stepIndex < stepCount; stepIndex++ {
  // 1. Build step-specific manifest (image, command, env from Steps[stepIndex])
  manifest := buildManifestFromRequest(req, typedOpts, stepIndex)

  // 2. Rehydrate workspace from base + diffs
  //    - Step 0: copy base clone only
  //    - Step k>0: copy base clone + apply diffs[0..k-1]
  workspace := rehydrateWorkspaceForStep(ctx, req, manifest, stepIndex, &baseClonePath)

  // 3. Execute step with healing (gate + mod container)
  result := executeWithHealing(ctx, runner, req, manifest, workspace, ...)

  // 4. Upload diff for this step (tagged with step_index)
  uploadDiffForStep(ctx, runID, diffGenerator, workspace, result, stepIndex)

  // 5. Cleanup workspace (except final step for MR push)
  if stepIndex < stepCount-1 {
    os.RemoveAll(workspace)
  }
}
```

### Backward Compatibility

The architecture maintains full backward compatibility with single-step runs:

- **Legacy specs**: Specs with top-level `mod` field (no `mods[]` array) create one stage with `step_index=NULL`
- **NULL step_index handling**: Diffs with `step_index=NULL` sort last in queries (`NULLS LAST`), ensuring legacy diffs don't interfere with multi-step ordering
- **Whole-run claims**: Nodes can still claim entire runs (legacy `ClaimRun` query) when no steps are available

### Example: 3-Step Run Across 2 Nodes

**Spec**:
```yaml
repo_url: https://github.com/example/project.git
base_ref: main
target_ref: feature-refactor
build_gate: mvn test
build_gate_healing:
  enabled: true
  max_attempts: 3
mods:
  - image: mods-plan:latest
    env:
      PLAN_STRATEGY: minimal
  - image: mods-openrewrite:latest
    env:
      RECIPE_GROUP: org.openrewrite.recipe
      RECIPE_ARTIFACT: rewrite-migrate-java
      RECIPE_VERSION: 3.20.0
  - image: mods-codex:latest
    env:
      CODEX_MODE: finalize
```

**Execution Flow**:

1. **Control Plane**: Creates 3 rows in `run_steps`:
   - `(run_id, step_index=0, status='queued')`
   - `(run_id, step_index=1, status='queued')`
   - `(run_id, step_index=2, status='queued')`

2. **Node A Claims Step 0**:
   - Fetches base clone: `git clone --depth 1 --branch main`
   - Rehydrates workspace: `copy(base_clone)` (no diffs yet)
   - Executes gate: `mvn test` (passes)
   - Executes mod: `docker run mods-plan:latest --dir /workspace`
   - Generates diff[0]: `git diff HEAD | gzip`
   - Uploads diff[0] with `step_index=0`
   - Updates `run_steps` row: `(step_index=0, status='succeeded', node_id=node-a)`

3. **Node B Claims Step 1**:
   - Fetches base clone: `git clone --depth 1 --branch main` (cached if same node)
   - Fetches diff[0] from control plane
   - Rehydrates workspace: `copy(base_clone) + apply(diff[0])`
   - Executes gate: `mvn test` (passes)
   - Executes mod: `docker run mods-openrewrite:latest --apply --dir /workspace`
   - Generates diff[1]: `git diff HEAD | gzip`
   - Uploads diff[1] with `step_index=1`
   - Updates `run_steps` row: `(step_index=1, status='succeeded', node_id=node-b)`

4. **Node A Claims Step 2**:
   - Base clone already cached from step 0
   - Fetches diff[0] and diff[1] from control plane
   - Rehydrates workspace: `copy(base_clone) + apply(diff[0]) + apply(diff[1])`
   - Executes gate: `mvn test` (passes)
   - Executes mod: `docker run mods-codex:latest --dir /workspace`
   - Generates diff[2]: `git diff HEAD | gzip`
   - Uploads diff[2] with `step_index=2`
   - Pushes branch to Git remote
   - Creates GitLab MR (if configured)
   - Updates `run_steps` row: `(step_index=2, status='succeeded', node_id=node-a)`

**Result**: Steps executed across 2 nodes with consistent workspace state reconstruction at each step.

### Reference Files

**Implementation**:
- `internal/nodeagent/execution_orchestrator.go` — Step loop and orchestration logic
- `internal/nodeagent/execution.go` — Rehydration helper (`RehydrateWorkspaceFromBaseAndDiffs`)
- `internal/nodeagent/difffetcher.go` — Diff download client
- `internal/worker/hydration/git_fetcher.go` — Shallow clone and caching
- `internal/server/handlers/nodes_claim.go` — Step-level claim handler
- `internal/store/queries/run_steps.sql` — Step claim and status queries
- `internal/store/queries/diffs.sql` — Diff storage and retrieval queries

**Schema**:
- `internal/store/migrations/007_diff_step_index.sql` — `diffs.step_index` column
- `internal/store/migrations/008_run_steps.sql` — `run_steps` table

**Documentation**:
- `ROADMAP.md` — Comprehensive delivery plan
- `docs/how-to/deploy-a-cluster.md` — Cluster deployment guide (includes multi-node architecture overview)
- `CHECKPOINT_MODS.md` — Current implementation state and examples

---

## What's Left (Test Exit Criteria)
1) Build Gate — API/CLI verifiable
   - Server: include `runs.stats` in `GET /v1/mods/{id}` or add `GET /v1/mods/{id}/stats`.
   - CLI: print gate status/duration in `mod inspect` when stats present.
   - Blast radius: server handlers (status), CLI mods/inspect; tests for both. ETA: ~0.5 day.
2) GitLab MR — push + open MR
   - Server‑side config: add secure storage of PAT (or reuse existing config), plumb to runner or a post‑apply step.
   - Node action: after ORW success, push branch and POST `projects/:id/merge_requests` (title, source, target) using PAT; record MR URL in ticket metadata and print in CLI.
   - Blast radius: nodeagent (post‑step), server config+handlers for MR metadata, CLI mods/inspect to show MR URL; docs update. ETA: ~1–1.5 days.

## Ready to Roll
- Server adds for diffs are implemented (list/download) and unit‑tested; roll out `ployd` to enable:
  - Update server binary on 45.9.42.212 and restart.
  - Then: `dist/ploy mod diffs <ticket> --download > changes.patch` to fetch the patch.
- Mod coords usage clarified in docs: env‑only (no JSON spec for coords).

Next commits landing in lab (already rolled for server endpoints):
- Server: diffs list/download handlers; artifact upload response now includes `cid`.
- CLI: `mod diffs` command; `--mod-command` accepts JSON array.

Open items to close the test:
- Expose `runs.stats.gate` in status API and print in CLI (`mod inspect`).
- GitLab MR path (PAT ingestion + push + `merge_requests` create; surface MR URL).

Proposal (required to proceed):
- Undrain at least worker‑b to allow claims, then re‑run the retained ORW probe.
  - Benefit: enables step 1 to execute and produce first‑mile logs and a retained container.
  - Risks: worker will start claiming queued work; ensure the lab queue is clean or use unique tickets.
  - Blast radius: control plane only; affects node scheduling; no code changes.
  - Time: ~2 minutes to undrain + verify claims; ~5–10 minutes to capture logs depending on pull time.

If approved, next concrete steps:
- POST `/v1/nodes/{id}/undrain` for worker‑b (mTLS), then re‑submit (or reuse `3ccec0b9-*`) and follow logs.
- After claim, capture:
  - `node_id` and IP from `GET /v1/mods/{ticket}`.
  - First 200 lines of logs via SSE; if empty, fetch `docker logs` on the node for the retained container labeled `com.ploy.run_id=<TICKET>`.
