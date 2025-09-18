E2E: Builder Logs Upload + Pointer (Lane D)
===========================================

Goal
----
- Exercise a Lane D (host Docker) build that emits clear log lines and then fails, in order to validate the controller’s builder logging feature:
  - events include `deployment_id`
  - `/v1/apps/:app/builds/:id/logs?lines=…` returns a non‑empty `.logs` tail
  - full logs are uploaded to SeaweedFS at `artifacts/build-logs/<id>.log` and accessible via `builder.logs_url`

Prerequisites
-------------
- Workstation env:
  - `PLOY_CONTROLLER` (e.g., `https://api.dev.ployman.app/v1`)
  - `TARGET_HOST` (VPS IP) for SSH fetching from SeaweedFS
- VPS has `/opt/hashicorp/bin/nomad-job-manager.sh` and SeaweedFS Filer reachable at `http://seaweedfs-filer.storage.ploy.local:8888`.

App Under Test
--------------
- `app/Dockerfile` prints canary lines and then intentionally fails the build:
  - proves logs were captured and uploaded before failure.

Scripts
-------
- `run.sh` — Submits the app tarball to the controller (Lane D, build_only). Saves headers/body and prints `BUILD_ID`.
- `fetch-logs.sh` — Fetches builder logs via API and full logs via SeaweedFS over SSH.
- `summarize.sh` — Produces `summary.txt` with pointers and a brief Docker build error tail.
- `test.sh` — One‑shot orchestration: run → fetch → summarize.

Usage
-----
1) Submit the build:
   - `cd tests/e2e/build-logs`
   - `APP_NAME=docker-fail-app PLOY_CONTROLLER=$PLOY_CONTROLLER ./run.sh`
   - Note the printed `BUILD_ID` and logs directory under `logs/<BUILD_ID>/`.

2) Fetch logs:
   - `APP_NAME=docker-fail-app BUILD_ID=<id> TARGET_HOST=$TARGET_HOST PLOY_CONTROLLER=$PLOY_CONTROLLER ./fetch-logs.sh`

3) Summarize:
   - `APP_NAME=docker-fail-app BUILD_ID=<id> ./summarize.sh`

4) Or run all:
   - `APP_NAME=docker-fail-app PLOY_CONTROLLER=$PLOY_CONTROLLER TARGET_HOST=$TARGET_HOST ./test.sh`

Outputs
-------
- `logs/<BUILD_ID>/headers.txt` — raw response headers (contains `X-Deployment-ID`).
- `logs/<BUILD_ID>/response.json` — submission response body (includes `builder.logs_key` / `builder.logs_url` on error).
- `logs/<BUILD_ID>/builder.logs.json` — API logs tail snapshot.
- `logs/<BUILD_ID>/builder.full.log` — full SeaweedFS log fetched via SSH.
- `logs/<BUILD_ID>/summary.txt` — pointers and tails for quick inspection.

Notes
-----
- If `builder.full.log` is empty, it likely indicates an early alloc failure or an upload race. Re‑run once; the controller now uploads a full log and includes a pointer on failure.
- You can slice platform API logs using `tests/e2e/deploy/fetch-logs.sh` with `START_TS_SOURCE=vps` if deeper context is needed.

Current State (as of latest run)
--------------------------------
- Harness created and exercised against the Dev API.
- Recent runs exercise the controller-hosted Docker build path. The sample app intentionally fails during `docker build`, ensuring the controller persists logs and responds with a `builder.logs_url` pointer even when the build aborts.

What to try next
----------------
- Re-run the scenario if needed:
  - `APP_NAME=docker-fail-app PLOY_CONTROLLER=$PLOY_CONTROLLER TARGET_HOST=$TARGET_HOST ./test.sh`
- Ensure `fetch-logs.sh` pulls both the API tail and the full SeaweedFS log; `summarize.sh` confirms that `.logs` contains the canary lines and that `builder.full.log` is non‑empty.

Success Criteria
----------------
- `X-Deployment-ID` present in headers after submission.
- `/v1/apps/:app/builds/:id/logs?lines=…` returns `.logs` containing canary lines.
- `artifacts/build-logs/<id>.log` exists in SeaweedFS and includes canary lines (verify via SSH fetch).
