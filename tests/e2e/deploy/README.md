# Lane D Deployment E2E

These end-to-end tests exercise the Docker-only lane after the 2025-09 consolidation. The harness generates minimal Go and Node.js applications on the fly, commits them into throwaway git repos, pushes with `ploy push -lane D`, waits for the async build, and verifies health.

## Running the suite

```bash
# Ensure the controller endpoint is available
export PLOY_CONTROLLER=https://api.dev.ployman.app/v1

# Optional: point to a local ploy binary
# export PLOY_CMD=/absolute/path/to/ploy

# Run the lane D matrix (Go + Node)
go test ./tests/e2e/deploy -tags e2e -v -run TestDeployLaneD -timeout 8m
```

Results are appended to repository-relative files so multiple runs can be compared later:

- `tests/e2e/deploy/results.jsonl` — machine-readable log (one JSON record per run).
- `tests/e2e/deploy/results.md` — quick Markdown table summarising lane/app/notes.

The helper test `TestWriteResultPaths` ensures these files resolve correctly even when Go runs tests from temporary directories.

## Inspecting logs

After an execution you can pull controller, Traefik, and runtime logs for an app via:

```bash
APP_NAME=<app-name> \
LANE=D \
LINES=400 \
TARGET_HOST=<vps-ip> \
BUILD_ID=<async-id-from-push> \
./tests/e2e/deploy/fetch-logs.sh
```

Key behaviours:
- Fetches `/apps/:app/status` and `/apps/:app/logs`.
- Streams the latest platform API and Traefik logs.
- When `BUILD_ID` is supplied, retrieves the controller-hosted builder logs (now produced by the host Docker build).
- If `TARGET_HOST` is provided, uses `nomad-job-manager.sh` to tail the `docker-runtime` task for the deployed job (`<app>-lane-d`).

## Sample applications

The harness produces two representative workloads:
- **Go** — multi-stage Dockerfile compiling a small HTTP service.
- **Node.js** — Node 20 Alpine image serving `/` and `/healthz`.

Each scenario validates that:
1. The controller accepts the async Lane D build and returns a deployment id.
2. The async status endpoint eventually reports `status=deployed`.
3. `/apps/:app/status` shows at least one running allocation.
4. The Nomad job is cleaned up via `ploy apps destroy --force`.

## Utilities

- `tests/e2e/deploy/fetch-logs.sh` — centralised log fetcher tuned for Lane D.
- `tests/e2e/deploy/write_result_paths_test.go` — guards repo-relative result paths.

These tests intentionally avoid remote GitHub scaffolding—everything is created locally so the suite can run in CI/VPS environments without additional credentials.
