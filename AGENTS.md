# Job/Run Investigation (Strict Short Runbook)

Use this flow for any "why did run/job fail?" request.

## Exec summary
- This repository is the orchestrator/control-plane, not the user project that failed to build/test.
- For job/run failure analysis, default to runtime evidence from the failed run/job first (status, logs, API-visible artifacts), then inspect orchestrator code only when evidence points there.
- Do not answer language/build-tool behavior questions from orchestrator internals unless explicitly asked to analyze orchestrator implementation.
- Active investigation target is cluster `round-leaf-6114`.
- Node SSH is available for investigation:
  - `tsh ssh v.v.kovalev@ploy-node-1.chi.t-oblako.ru`
  - Use `sudo` if root privileges are necessary.

## 1) Use control-plane endpoints first
- Do not start with broad repo searches.
- Use `~/.config/ploy/default` for the `round-leaf-6114` server URL and token.
- Treat API/CLI output as the first source of truth.
- If API-visible logs/artifacts are empty, ambiguous, or insufficient, SSH to the node and inspect runtime evidence directly.
- Never paste auth tokens into responses or shell output summaries.

## 2) Designated debug endpoints
- Run status:
  - `GET /v1/runs/<run-id>/status`
  - CLI: `ploy run status <run-id> --json`
- Run lifecycle events:
  - `GET /v1/runs/<run-id>/logs`
- Job status:
  - `GET /v1/jobs/<job-id>/status`
- Job logs:
  - `GET /v1/jobs/<job-id>/logs`
  - CLI: `ploy job log --format raw <job-id>`
- Repo job list:
  - `GET /v1/runs/<run-id>/repos/<repo-id>/jobs`
- Repo artifact list:
  - `GET /v1/runs/<run-id>/repos/<repo-id>/artifacts`
- Artifact metadata/download:
  - `GET /v1/artifacts/<artifact-id>`
  - `GET /v1/artifacts/<artifact-id>?download=true`
- Diff list/download:
  - `GET /v1/runs/<run-id>/repos/<repo-id>/diffs`
  - `GET /v1/runs/<run-id>/repos/<repo-id>/diffs?download=true&diff_id=<uuid>`

## 2.1) Direct DB fast path for simple status questions
- If question is a single factual check (for example: "what is this job status?"), answer from `/v1/jobs/<job-id>/status` or `ploy run status <run-id> --json` first, before collecting full runtime evidence.
- Only continue to logs and artifacts when the API result is missing, ambiguous, or conflicts with observed behavior.

## 2.2) Node maintenance and free-space checks
- Prefer direct node inspection over inferred state when SSH is available.
- If SSH is unavailable, use node diagnostics instead of host shell commands.
- Free-space data is available from:
  - `GET /v1/nodes` for the selected constrained storage aggregate.
  - `GET /v1/nodes/<node-id>/diagnostics` for `node.details.storage.paths`, covering `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`, `PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`.
- Node maintenance is host-owned by deploy `systemd` services:
  - `ploy-node-auth-refresh.service`
  - `ploy-node-update.service`
  - `ploy-node-cleanup.timer`
- Historical node action rows remain readable with `GET /v1/nodes/<node-id>/actions?limit=N`.
- Node image pulls read Docker auth only from `PLOY_DOCKER_AUTH_CONFIG_FILE`; on registry unauthorized, `PLOY_DOCKER_AUTH_REFRESH_CONTAINER` can refresh auth once before a single retry.

## 3) Minimum required evidence
- `ploy run status <run-id> --json`
- `ploy job log --format raw <job-id>`
- `GET /v1/jobs/<job-id>/status`
- `GET /v1/runs/<run-id>/repos/<repo-id>/jobs`
- `GET /v1/runs/<run-id>/repos/<repo-id>/artifacts`
- Download relevant artifact bundles with `GET /v1/artifacts/<artifact-id>?download=true` when present.

## 3.1) Fast route by exit code (first triage branch)
- If `exit_code = -1`: treat as ploy/orchestrator-internal failure first.
  - Prioritize job status, raw job logs, repo job list, and artifact bundles for that `job_id`.
  - Typical scope: pre-container/setup/population failures (for example input/materialization issues). If API evidence is insufficient, state the unavailable evidence explicitly.
- If `job_type = heal` and `exit_code = 1`: treat as job payload/tooling failure first.
  - Prioritize runtime artifacts from `*_repo-artifacts.bin`, especially `artifacts/<job-id>/out/amata/runs/**` (`events.ndjson`, `snapshot.json`, provider outputs when present).
  - Only pivot to ploy internals after runtime evidence is insufficient/inconsistent.

## 4) Artifact retrieval path (no re-discovery)
- Always fetch artifacts with:
  - `ploy mig fetch --run <run-id> --artifact-dir <dir>`
- If `ploy mig fetch` does not expose the needed bundle, use:
  - `GET /v1/runs/<run-id>/repos/<repo-id>/artifacts`
  - `GET /v1/artifacts/<artifact-id>?download=true`
- If needed, unpack `*_repo-artifacts.bin` (`tar -xzf ...`) and inspect:
  - `artifacts/<job-id>/out/amata/runs/*/events.ndjson`
  - `artifacts/<job-id>/out/amata/runs/*/snapshot.json`
  - `artifacts/<job-id>/out/heal.json`
- For heal/payload analysis, correlate artifact behavior with MIG sources in:
  - `/Users/v.v.kovalev/@scale/ploy-lib/migs`

## 5) Important caveat
- `/out` bundle may not include per-step provider files (`stderr.txt`, `transcript.txt`, `provider-metadata.json`).
- If missing there and absent from the repo-local artifacts tree, report clearly: exact provider-internal error is unrecoverable post-factum.

## 6) Repo-local artifacts
- Jobs write durable repo-local artifacts under:
  - `$PLOYD_CACHE_HOME/runs/<run-id>/repos/<repo-id>/artifacts`
  - per-job files live in `<job-id>/{in,out,stdout.log,stderr.log,diff.patch}`
- The node uploads the full `artifacts/` tree on job failure/error and successful `post_gate`.
- With SSH access, inspect the node-side repo-local artifact tree directly when uploaded bundles are insufficient.
- If SSH is unavailable and a required file exists only on the node filesystem, report that limitation explicitly.

## 7) Response contract
- Separate:
  - immediate failure cause (this job)
  - underlying product/build failure (if different)
- State explicitly what is proven vs unavailable.

## 8) Maintenance note
- Keep this runbook's investigation routes current as failure patterns evolve (especially exit-code routing and temp-preservation paths).
