# Checkpoint — Mods E2E (ORW) Current State (Nov 5, 2025)

## Summary
We’ve unblocked the control plane and workers, implemented proper container retention and log streaming, and verified the container runtime path with a self‑test. The OpenRewrite (ORW) step still fails quickly with exit code 1 and no mod logs exposed to the stream, so we’re adding a targeted retained probe on the ORW image to capture first‑mile failure details.

Update this document as you go.

## What’s Fixed
- Container lifecycle: disabled Docker AutoRemove and added explicit delete after logs are fetched; added `--retain-container` to keep containers for inspection.
- SSE logs: node uploads gzipped chunks; server gunzips and publishes per‑line SSE; log ingest routes now publish to SSE.
- Diagnostics in status: `GET /v1/mods/{id}` now includes `metadata.node_id` (which worker claimed the run) and `metadata.reason` (e.g., `exit code 1`).
- Workers re‑added: re‑provisioned worker‑b (46.173.16.177) and worker‑c (81.200.119.187) with correct mTLS config; claims work again.
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
  - worker‑b (46.173.16.177): claims ORW; recent runs end `exit code 1`; Docker reachable interactively.
  - worker‑c (81.200.119.187): self‑test succeeded; Docker reachable.
- Logging: SSE pipeline is enabled end‑to‑end; absence of logs on ORW indicates the container likely isn’t producing (or step fails before start), not a transport issue.

## Notes
- All retention and log‑fanout changes are unit‑tested; control plane and node binaries have been rebuilt and deployed.
- Containers created by steps carry `com.ploy.run_id=<TICKET>` for easy discovery; removal is explicit and skipped when retained.

---

## Updates — 2025‑11‑04

What I attempted (literal to Next Actions):
- Built CLI (`dist/ploy`).
- Pointed CLI to control plane: `PLOY_CONTROL_PLANE_URL=https://45.9.42.212:8443` (mTLS via default descriptor).
- Submitted retained ORW probe twice using `docker.io/iwtormb/mods-openrewrite:latest`, with `--mod-command` printing env/versions, listing `/workspace`, and invoking the Rewrite plugin (`-X`).
  - Tickets: `fbd8ea42-e9b4-4f2f-bb22-34d573729de3` and `3ccec0b9-a83c-4fcb-b310-a99b6a1b6ccc`.

Observed blockers (initial):
- Both workers reported drained=true via `GET /v1/nodes`.
  - worker‑c 81.200.119.187 (drained=true)
  - worker‑b 46.173.16.177 (drained=true)
- Result: new tickets remained `pending/queued`; no node claims, no SSE logs.

Actions taken after approval:
- Undrained worker‑b via API; then attempted `ploy rollout nodes --selector worker-b` to refresh `ployd-node`.
- Rollout failed at the heartbeat confirmation step, but worker‑b resumed heartbeating (hb updated to ~23:24Z).
- Undrained both worker‑b and worker‑c; current state:
  - worker‑b 46.173.16.177 — drained=false, last_heartbeat recent (23:24Z)
  - worker‑c 81.200.119.187 — drained=false, last_heartbeat stale (22:28Z)

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
- node_id: `28587647-682f-4ab1-b5a4-a2d036a35a20` (worker‑b 46.173.16.177)
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

Current gaps vs test goals:
1) Build Gate confirmation — We execute the Java build gate (`mvn test`) inside the node runner (Docker). Timings are persisted in run `stats` (`build_gate_duration_ms`), but `GET /v1/mods/{id}` does not expose `stats` yet.
   - Evidence now: runs that succeed did not report gate errors; container exited 0; end‑to‑end timings uploaded (DB `runs.stats`).
   - To make this testable via API/CLI, expose `stats` in `GET /v1/mods/{id}` (or add `GET /v1/mods/{id}/stats`) and print "Gate: passed in X ms" in CLI.
   - Optional: attach a small `build-gate.log` artifact or emit SSE lines from gate executor.
2) GitLab MR — Branch push + MR creation not wired yet (PAT handling is TODO per docs).
   - Minimal path: accept PAT (CLI flag `--gitlab-pat` or env `PLOY_GITLAB_PAT`), propagate securely to the node runner, then:
     - `git config user.*`, ensure branch exists (create or reuse target ref), `git push` with PAT, call GitLab REST to open MR (store MR URL in ticket `metadata` and print in CLI).
   - Preferred path: store PAT on control plane (mTLS‑guarded config) and call GitLab API server‑side to avoid PAT on nodes; node pushes via deploy key or PAT injected only for git remote.
   - Both require small code changes; see plan below.

## What’s Left (Test Exit Criteria)
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
