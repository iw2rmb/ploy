# End-to-End Test Harness

This directory hosts the Go-based E2E harness that exercises Mods workflows and the
controller API against real infrastructure. The tests are opt-in: they are guarded
by the `e2e` build tag and skip automatically unless the required environment is
wired up (controller URL, test repositories, optional VPS host).

## What currently runs

- `stack_e2e_test.go` – probes `/health` and `/version` on the controller
  (`PLOY_CONTROLLER`).
- `api_platform_e2e_test.go` – checks platform status and Traefik log snapshots via
  the public API (`/platform/api/status`, `/platform/traefik/logs`).
- `mods_workflows_test.go` – launches a Mods Java migration workflow and waits for a
  completed run. Falls back to invoking the controller directly if the CLI fails to
  return a `mod_id`.
- `vps_e2e_test.go` – reuses the workflow harness against a remote VPS target and
  optionally spawns concurrent runs when `TARGET_HOST` is set.
- `skip_test.go` – placeholder that keeps `go test ./tests/e2e` green when the `e2e`
  tag is not provided.

All tests rely on the helpers in `framework.go`, `types.go`, and `parsing.go`. The
framework shells out to a local `bin/ploy` (or `bin/ploy-linux` for VPS) and parses the
JSON Mods output to assert on merge request URLs, branch names, durations, etc.

## Running the suite

```
# Build the CLI binaries first (host + optional linux target)
make build-cli
make build-cli-linux   # required when running against TARGET_HOST

# Run the e2e package with the build tag enabled
PLOY_CONTROLLER=https://api.dev.ployman.app/v1 E2E_REPO=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git E2E_BRANCH=e2e/success go test -tags e2e ./tests/e2e -v
```

Set `TARGET_HOST` to route execution through the remote VPS wrapper; otherwise the
harness expects local Nomad/Consul/Seaweed instances.

## Subfolder workflows

- `build-logs/` — scripts (`run.sh`, `fetch-logs.sh`, `summarize.sh`) exercise a deliberately failing Lane D Docker build and verify the controller exposes both the short log tail and the full SeaweedFS artifact. 
  See `tests/e2e/build-logs/README.md` for usage and success criteria.
- `deploy/` — Go-based lane D deploy tests that scaffold throwaway apps, push them via `ploy push`, and capture results in `results.jsonl`/`results.md`. 
  Contains helpers like `fetch-logs.sh` to pull controller + runtime logs; documented in `tests/e2e/deploy/README.md`.
- `mods/` — shell workflows for the Mods OpenRewrite flows (Java 11→17, LLM planning).
  Useful when debugging outside Go tests; details live in the per-folder README files.

### Key environment variables

| Variable | Purpose |
| --- | --- |
| `PLOY_CONTROLLER` | Base URL for the controller used by stack/platform checks and Mods fallbacks. Required for most tests. |
| `E2E_REPO` / `E2E_BRANCH` | Override the Git repository and branch the migration workflow clones. Defaults match the java11→17 sample repo. |
| `TARGET_HOST` | SSH target (e.g. `45.12.75.241`) that switches the CLI wrapper to the VPS binaries. Optional. |
| `E2E_LOG_CONFIG` | When set to `1`, dumps the generated Mods YAML and collects controller logs for the run. |

Additional controller credentials (PATs, Nomad tokens, etc.) must be provided via the
standard CLI environment variables if the target installation requires them.

## Logs and artifacts

Successful runs emit the Mods execution ID and merge request URL. When a workflow
fails, `runModsCollector` harvests controller logs into `tests/e2e/build-logs/`. The
`build-logs` scripts (`run.sh`, `summarize.sh`) can be used to replay or collate those
artifacts after a run.

## Current status / gaps

- Tests are **not** part of CI; they are meant for manual or pre-release validation.
- The harness assumes externally managed infrastructure – it does not provision the
  controller, Nomad, GitLab tokens, or SeaweedFS.
- Assertions still expect successful runs; when the platform is in a RED phase, the
  tests provide diagnostics but will fail hard.
- Some workflows still rely on fallbacks (controller-side execution) while the CLI
  matures; watch the log output for `Continuing despite CLI error`.

See `../mods/README.md` for runtime knobs and `../../Makefile` for helper targets that
bundle common environment exports.
