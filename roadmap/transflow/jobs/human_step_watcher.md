## Human-Step Watcher (Interface)

Watches for human involvement on the workflow branch (e.g., a push, MR merge, or specific commit message), then triggers a build check to validate success.

### Inputs
- Env:
  - `REPO_URL` — HTTPS URL with token in env or credential store
  - `BRANCH` — workflow branch to watch (e.g., `workflow/<id>/<timestamp>`)
  - `POLL_INTERVAL` — e.g., `15s`
  - `TIMEOUT` — e.g., `2h`
  - `CONTROLLER_URL` — build API base (e.g., `${PLOY_CONTROLLER}`)
  - `APP_NAME` — build app name (e.g., `tfw-<id>-<timestamp>`)
  - `LANE` — optional lane override
  - `RUN_ID` — for correlation
- Args: none required

### Behavior
1) Poll Git remote for new commits to `BRANCH` (compare last seen commit hash).
2) On new commit:
   - Trigger build check (`POST /v1/apps/:app/builds?env=dev[&lane=...]`) by streaming a tar of the branch.
   - If build passes (HTTP 200): write a branch record with `status=success` and include build metadata; exit 0.
   - Else continue polling until `TIMEOUT`.
3) On `TIMEOUT`: write `status=timeout`; exit non‑zero.
4) On cancellation signal (SIGTERM): write `status=canceled`; exit quickly.

### Outputs
- stdout JSON: `{ "ok": true, "status": "success|timeout|canceled" }`
- Optional file `out/branch.json` conforming to `platform/nomad/transflow/schemas/branch_record.schema.json`.

### Security
- Use env-injected token only; do not accept tokens in YAML.
- Read-only operations until build trigger. Build uses controller API; no direct deploys.
