# `ploy run` command refactor

## Summary

Refactor `ploy run` from the current flag-heavy submit/inspect/pull surface into a small positional command family:

```bash
ploy run <spec-path> [ <repo-path=.> {--apply} | <namespace/repo{:<branch=master|sha>}> ] { --pull {<artifacts-path=OS tmp>} }
ploy run ls {[ | <path=.> | <namespace/repo{:<sha|branch>}> ]} {--limit X} {--offset Y}
ploy run cancel <run-id>
ploy run apply <run-id> {<path=.>} {--force}
ploy run pull <run-id> {<artifacts-path=OS tmp>}
ploy run status <run-id> {--json | --follow}
```

The new contract removes submit-time flag composition, removes the old diff-oriented `run pull`, replaces `run patch` with `run apply`, removes `run start`, removes `run logs`, and keeps `run status`.

## Scope

In scope:

- `internal/cli/run` command tree and handlers.
- `ploy job log` default-format adjustment.
- Shared CLI helpers needed to resolve local repo identity, remote repo specs, artifact directories, and patch application.
- Removal of `GET /v1/runs/{run_id}/logs` and its CLI client path.
- CLI tests and app-level help/golden outputs for the new `ploy run` surface.
- Public command docs that currently describe the removed `ploy run` flags and subcommands.
- Autocomplete regeneration after the Cobra tree changes.

Out of scope:

- Refactoring `ploy mig run` or `ploy mig pull`.
- Removing unrelated server endpoints that are still needed by nodes, status rendering, or other commands.
- Changing the run/job execution graph.
- Adding compatibility aliases for removed flags or commands.

No backward compatibility is required. Removed commands and flags should fail through normal Cobra unknown command/flag behavior.

## Why This Is Needed

The current `ploy run` surface exposes internal execution details as user-facing flags: repository URL, base ref, target ref, follow caps, retry counts, job image, job command, job envs, JSON output, artifact directories, old pull mechanics, patch download mechanics, and manual start. That makes the command ambiguous: it mixes run submission, job execution tuning, artifact download, and diff application.

The target contract makes one path primary: submit a spec against a local or remote repo, optionally wait for final artifacts, and optionally apply a successful patch to a local repo. Everything else is either implementation detail, existing status inspection, or an operation with a clearer name.

## Goals

- Make `ploy run <spec-path>` the only submit entrypoint under `ploy run`.
- Treat the spec file or spec directory as the first positional argument.
- Resolve omitted repo input from the current directory, with a hard failure outside a git worktree.
- For local repo input, submit exactly `HEAD`; staged and unstaged changes are ignored.
- For remote repo input, accept `namespace/repo`, `namespace/repo:<branch>`, or `namespace/repo:<sha>`, defaulting the ref to `master`.
- Keep artifact download explicit through `--pull` on submit and `ploy run pull <run-id>`.
- Make patch application explicit through `--apply` on submit and `ploy run apply <run-id>`.
- Remove user-facing flags that override job/spec internals.
- Keep `ploy run status <run-id> {--json | --follow}` as the status command.

## Non-Goals

- No compatibility shims for root submit flags: `--repo`, `--base-ref`, `--target-ref`, `--spec`, root `--json`, `--job-env`, `--job-image`, `--job-command`, `--artifact-dir`, `--cap`, `--cancel-on-cap`, or `--max-retries`.
- No legacy rejection code that enumerates previous shapes.
- No `ploy run patch` alias.
- No reuse of the current `ploy run pull` branch-creation workflow for artifact download.
- No manual `ploy run start` path.
- No `ploy run logs` command.

## Current Baseline (Observed)

- Root CLI wiring adds `runcli.NewCommand()` from `internal/cli/run` at `internal/cli/app/root.go:48-51`.
- `internal/cli/run/run_commands.go:18-59` builds the current Cobra tree. The root command only submits when one of `--repo`, `--base-ref`, `--target-ref`, or `--spec` is present; otherwise it prints help.
- Root submit flags are registered at `internal/cli/run/run_commands.go:38-50`: `--repo`, `--base-ref`, `--target-ref`, `--spec`, `--follow`, `--cap`, `--cancel-on-cap`, `--max-retries`, `--job-env`, `--job-image`, `--job-command`, `--artifact-dir`, and `--json`.
- Current subcommands are wired at `internal/cli/run/run_commands.go:52-58`: `ls`, `cancel`, `start`, `status`, `logs`, `pull`, and `patch`.
- Current list uses `--limit` and `--offset`, then calls `migs.ListBatchesCommand` in `internal/cli/run/run_list.go:22-57`.
- Current submit requires `--repo`, `--base-ref`, `--target-ref`, and `--spec` in `internal/cli/run/run_submit.go:51-65`.
- Current submit builds `domainapi.RunSubmitRequest{repo_url, base_ref, target_ref, spec, created_by}` and posts it to `POST /v1/runs` in `internal/cli/run/run_submit.go:93-103` and `internal/cli/run/run_submit.go:272-328`.
- Current spec loading uses `specpayload.Build` in `internal/cli/run/run_submit.go:143-199`. `specpayload.Build` reads the path as a file with `common.ReadFileRooted` at `internal/cli/specpayload/mig_run_spec.go:551-560`; it does not currently implement the target "directory containing mig.yaml" behavior.
- Current submit follow mode is owned by `followRunSubmit` and uses `runs.FollowRunCommand` with cap/cancel/retry controls in `internal/cli/run/run_submit.go:202-269`.
- Current artifact download is only attached to successful follow mode when `--artifact-dir` is set in `internal/cli/run/run_submit.go:124-130`.
- `DownloadRunArtifacts` fetches status, resolves artifact CIDs through `GET /v1/artifacts?cid=...`, downloads via `GET /v1/artifacts/{id}?download=true`, and writes `manifest.json` in `internal/cli/run/run_artifact.go:31-196`.
- Current `ploy run logs` is registered as a subcommand in `internal/cli/run/run_commands.go:124-142` and implemented by `internal/cli/run/run_logs.go`.
- Current `GET /v1/runs/{run_id}/logs` is registered at `internal/server/handlers/register.go:101` and served by the run lifecycle SSE handler in `internal/server/handlers/events.go`.
- Current `ploy job log` registers `--format` with default `structured` in `internal/cli/app/commands_job.go`; `internal/cli/job/log.go` also defaults an empty format to structured.
- Current `ploy run pull` delegates to `internal/cli/pull.HandleRunPull` from `internal/cli/run/run_commands.go:145-166`. That old flow requires a git worktree, requires a clean tree, resolves `origin`, calls `POST /v1/runs/{run_id}/pull`, fetches refs, creates a branch, downloads diffs, and applies them at `internal/cli/pull/run_pull.go:57-219`.
- Current `ploy run patch` downloads gzip patch bytes without applying them through `RunPatch` in `internal/cli/run/run_patch.go`.
- Current `ploy run start` calls `runs.StartCommand` and `POST /v1/runs/{id}/start` through `internal/cli/run/run_start.go:20-49`.
- The server already has a background batch scheduler whose comment says it eliminates manual `POST /v1/runs/{id}/start` calls at `internal/store/batchscheduler/batch_scheduler.go:1-5`.
- The server `POST /v1/runs` handler currently requires `repo_url`, `base_ref`, `target_ref`, and `spec` at `internal/server/handlers/runs_submit.go:19-60`, resolves the source commit before storing rows at `internal/server/handlers/runs_submit.go:64-77`, and stores `source_commit_sha` plus `repo_sha0` on `run_repos` at `internal/server/handlers/runs_submit.go:132-141`.
- `RepoURL` currently accepts only `https://`, `ssh://`, and `file://` values in `internal/domain/types/vcs.go:10-45` and validates that shape at `internal/domain/types/vcs.go:84-92`. The target `namespace/repo` syntax therefore needs control-plane-owned expansion before calling current API shapes.
- `RunSubmitRequest` currently includes public `target_ref` in `internal/domain/api/run_submit.go:9-20`, so removing `--target-ref` from the CLI still leaves either a server contract cleanup or a hidden CLI-generated value to resolve.

## Target Contract

### `ploy run <spec-path> ...`

Arguments:

- `<spec-path>` is required.
- If `<spec-path>` is a directory, resolve it to `<spec-path>/mig.yaml`.
- If `<spec-path>` is a file, use that file directly.
- The file must be valid YAML or JSON accepted by the current spec contract.

Repo selection:

- If no repo argument is provided, use `.`.
- A repo argument that resolves to an existing local path is a local repo path.
- A repo argument that does not resolve to a local path is parsed as remote shorthand: `namespace/repo`, `namespace/repo:<branch>`, or `namespace/repo:<sha>`.
- Remote shorthand default ref is `master`.
- Local repo mode must run git commands in the selected path, not in the original process CWD.
- Local repo mode must hard fail if the selected path is not inside a git worktree.
- Local repo mode must resolve the repo URL from `origin` and the source ref from `HEAD`.
- Local repo mode must ignore staged and unstaged changes. It does not package local diffs.
- Remote mode must ask the control plane to expand shorthand into a full repository URL before submitting. The default host/scheme is cluster-owned, not a CLI literal or local descriptor field.

Submit behavior:

- Submit one single-repo run.
- The user does not provide `base_ref` or `target_ref`.
- The source commit must be the selected commit:
  - local mode: current `HEAD` SHA;
  - remote branch mode: server-resolved tip of the branch;
  - remote SHA mode: exact SHA, without interpreting it as a branch.
- The run should be followed to terminal state when either `--pull` or `--apply` is provided, because both operations depend on successful completion.
- Without `--pull` and without `--apply`, submission returns after the control plane accepts the run and prints the run ID in human text.

Patch application:

- `--apply` is allowed only for local repo mode.
- `--apply` waits for the run to succeed, downloads accumulated diffs for the run repo, and applies them to the selected local repo path.
- `--apply` must not create or switch branches.
- `--apply` must not require the old `target_ref` user input.
- Before applying, fail if the selected local repo has any staged or unstaged diff against `HEAD`.
- Before applying, fail if local `HEAD` does not match the run repo `source_commit_sha`.
- `--force` bypasses only the local `HEAD` versus run source SHA guard.
- `--force` never bypasses the clean-state guard.

Artifact pulling:

- `--pull` on submit means "download final artifacts after a successful run".
- `--pull=<path>` on submit downloads final artifacts to the provided path.
- Bare `--pull` downloads final artifacts to a new OS temp directory.
- `ploy run pull <run-id>` means "download final artifacts for an existing run".
- If an artifacts path is omitted, create a new OS temp directory.
- Output must print the chosen artifact directory.
- Artifact download should reuse the existing artifact API path and manifest shape from `DownloadRunArtifacts`.

### `ploy run ls`

Arguments:

- No repo selector lists recent runs globally.
- A local path selector lists runs for the repo resolved from that path.
- A remote shorthand selector lists runs for the resolved repository.

Pagination:

- Keep the current `--limit` and `--offset` flag names.
- `--limit` must be bounded by the current server/client maximum.
- `--offset` remains 0-based and maps directly to the existing list API.

### `ploy run apply <run-id> {<path=.>} {--force}`

- Applies the successful run patch to the git worktree at `<path>`.
- Defaults `<path>` to `.`.
- Replaces `ploy run patch`.
- Resolves the run repo by local repo URL, not by user-provided repo ID.
- Fails if the path is not a git worktree.
- Fails if `git -C <path> diff --quiet HEAD --` reports any staged or unstaged diff.
- Resolves local `HEAD` with `git -C <path> rev-parse HEAD`.
- Resolves the run source SHA from the matched run repo `source_commit_sha`.
- Fails if local `HEAD` differs from run `source_commit_sha`.
- Allows the SHA mismatch only with `--force`.
- `--force` does not override local diff checks.
- Downloads accumulated diffs and applies them with `git apply`.
- Does not expose `--repo-id`, `--repo-url`, `--diff-id`, `--output`, `--origin`, or `--dry-run`.

### `ploy run pull <run-id> {<artifacts-path=OS tmp>}`

- Downloads final artifacts into the provided path or a new OS temp directory.
- Replaces the old diff/branch workflow under `ploy run pull`.
- Does not inspect a git worktree.
- Does not call `POST /v1/runs/{run_id}/pull`.
- Does not create branches.
- Does not apply patches.

### `ploy run cancel <run-id>`

- Keep the current cancellation behavior.
- Remove nonessential options from the public surface unless there is a current product need for them. The proposed target syntax has no `--reason`.

### `ploy run status <run-id> {--json | --follow}`

- Keep `ploy run status`.
- Keep the existing human status renderer.
- Keep `ploy run status --json` and the existing JSON status renderer.
- Add `ploy run status <run-id> --follow`.
- `--follow` is for running runs and prints the same live status view currently used by submit follow mode.
- `--follow` exits when the run reaches a terminal state and renders the final status snapshot.
- `--json` and `--follow` are mutually exclusive.
- Root submit `--json` is still removed; status JSON is a retained inspection capability.

### `ploy job log`

- Keep `ploy job log [--follow|-f] [--format <raw|structured>] <job-id>`.
- Change the default format to `raw`.
- `--format structured` remains available for callers that need timestamp/stream-prefixed records.
- The command should emit raw log lines when `--format` is omitted.

## Implementation Notes

### Command Tree

- Rewrite `internal/cli/run/run_commands.go` so root `Use` is `run <spec-path> [<repo>]`.
- Remove root flags for `--repo`, `--base-ref`, `--target-ref`, `--spec`, `--follow`, `--cap`, `--cancel-on-cap`, `--max-retries`, `--job-env`, `--job-image`, `--job-command`, `--artifact-dir`, and `--json`.
- Add only the root flags required by the target contract: `--apply` and a string `--pull` flag with `NoOptDefVal` so both bare `--pull` and `--pull=<path>` are accepted.
- Replace subcommand registration with only `ls`, `cancel`, `status`, `apply`, and artifact-oriented `pull`.
- Delete `newStartCommand`, `newLogsCommand`, and `newPatchCommand`.

### Status Path

- Extend `StatusOptions` with `Follow bool`.
- Keep `JSONOut bool` for `ploy run status --json`.
- Reject `--json --follow` together.
- For non-follow status, keep the existing `runcmd.GetRunReportCommand` plus human/JSON renderers.
- For `--follow`, render the same live status view as current submit follow mode, but drive it from repeated run report/status fetches instead of `GET /v1/runs/{run_id}/logs`.
- Refactor any reusable renderer out of `runs.FollowRunCommand` if needed; do not keep the run-log SSE endpoint only for follow rendering.
- Do not add `--cap`, `--cancel-on-cap`, or `--max-retries` to `run status --follow`; use command-owned defaults.

### Job Log Path

- Change the Cobra flag default in `internal/cli/app/commands_job.go` from `structured` to `raw`.
- Change `internal/cli/job/log.go` so an empty `LogOptions.Format` defaults to `logs.FormatRaw`.
- Keep validation restricted to `raw` and `structured`.
- Update tests that assume the implicit default is structured; keep explicit structured-format coverage.

### Submit Path

- Replace `SubmitOptions` with a positional model:
  - `SpecPath`
  - `RepoSelector`
  - `Apply bool`
  - `PullArtifacts bool`
  - `ArtifactsPath string`
  - output writers.
- Replace `validateRunSubmitFlags` with validation of positional args and mutually exclusive local/remote semantics.
- Move spec path resolution before `specpayload.Build`:
  - directory -> `mig.yaml`;
  - file -> itself.
- Stop passing CLI overrides into `specpayload.Build`; `migEnvs`, `migImage`, and `migCommand` should be empty because those flags are removed.

### Repo Resolution

Create a small internal resolver, owned by `internal/cli/run`, that returns:

```go
type resolvedRunRepo struct {
    Mode       string
    Worktree   string
    RepoURL    string
    Ref        string
    CommitSHA  string
    IsLocal    bool
}
```

Local mode:

- Resolve absolute worktree path.
- Run `git -C <path> rev-parse --is-inside-work-tree`.
- Run `git -C <path> rev-parse HEAD`.
- Run `git -C <path> remote get-url origin`.
- Do not read `git status` for submit.

Remote mode:

- Parse the last colon as the optional ref delimiter only when the value is not a URL and not a local path.
- Default missing ref to `master`.
- Expand `namespace/repo` through a control-plane endpoint before submit.
- The endpoint returns the canonical credential-free repo URL and the resolved ref/commit interpretation.
- The CLI does not store or infer the repo host. It only parses shorthand enough to separate `namespace/repo` from the optional `:<ref>` suffix.
- If the ref is a full SHA, submit the SHA as the selected source. Otherwise submit the branch ref and let the server resolve the SHA.

### Remote Shorthand Expansion

- Add a control-plane endpoint for repo shorthand expansion, for example `POST /v1/repos/resolve`.
- Request shape should carry `selector` and optional `ref`, where `selector` is `namespace/repo`.
- Response shape should return at least `repo_url`, `ref`, and whether `ref` is a commit SHA.
- The endpoint must return credential-free repository URLs. Authentication remains server-owned at execution time.
- The endpoint is the only authority for default host, URL scheme, and any cluster-specific repository namespace rules.
- The CLI must not read `PLOY_GITLAB_DOMAIN`, infer from the control-plane address, or add repo host fields to the local cluster descriptor for this feature.

### Server Contract Pressure

The clean target is to remove public `target_ref` from run submission because the new command has no target branch concept. There are two implementation paths:

- Preferred: introduce a new current `RunSubmitRequest` shape with `repo_url`, `ref` or `commit_sha`, and `spec`; remove `target_ref` from submit and avoid storing a generated target branch for `ploy run`.
- Smaller implementation: keep current `POST /v1/runs` temporarily and set internal `base_ref`/`target_ref` from the resolved source ref, while making `target_ref` non-user-facing. This is easier but leaves a misleading database field and should not become the long-term contract.

Because this DD is for the current command contract and no backward compatibility is required, the preferred path should be implemented unless the server-side migration is judged too large for the first implementation slice.

### Apply Path

- Replace `RunPatch` with `RunApply`.
- Move reusable git helpers from `internal/cli/pull` into a neutral package or into `internal/cli/run` if only `run apply` uses them after the old `run pull` is removed.
- Resolve the run repo by local repo URL through existing `POST /v1/runs/{run_id}/pull` or a renamed server resolver. The endpoint name is now semantically awkward for apply, but the behavior is still repo execution resolution.
- Use the matched run repo's `source_commit_sha` as the authoritative run source SHA.
- Check local clean state with `git -C <path> diff --quiet HEAD --`; this catches both staged and unstaged changes.
- Check local source identity with `git -C <path> rev-parse HEAD`.
- Fail on local `HEAD` mismatch unless `--force` is set.
- Even with `--force`, fail on any staged or unstaged diff.
- Download accumulated diffs through the existing `GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=...&accumulated=true` path.
- Apply patches using `git -C <path> apply`.
- Do not create branches, switch branches, detach HEAD, or otherwise move the local checkout before applying.
- Do not keep the `run patch` gzip-byte writer or its `--output -` stdout contract.

### Artifact Pull Path

- Reuse `DownloadRunArtifacts`, but make the caller own default temp directory creation.
- `ploy run pull <run-id>` should be a thin wrapper around artifact download.
- Root `ploy run ... --pull` should call the same helper after successful completion.
- Consider renaming `run_artifact.go` comments from `mig_run_artifact` to `run_artifact` while touching the file.

### List Path

- Keep `ListOptions` pagination fields as `{Limit, Offset}` and add repo selector state separately.
- For local path or remote shorthand selectors, resolve a normalized repo URL and call a repo-scoped list endpoint. Existing `GET /v1/repos/{repo_id}/runs` requires a repo ID, not a URL, so implementation must either:
  - add a repo lookup/list-by-url client path; or
  - extend the existing run list API with a `repo_url` filter.
- Keep output human-readable and tabular.

### Removals

Remove these CLI files or collapse their contents:

- `internal/cli/run/run_start.go`.
- `internal/cli/run/run_logs.go`.
- `internal/cli/run/run_patch.go`.
- `internal/cli/run/run_patch_test.go`.
- Server route and handler support for `GET /v1/runs/{run_id}/logs`.
- OpenAPI/docs entries for `GET /v1/runs/{run_id}/logs`.
- Old `run pull` command wrapper in `internal/cli/run/run_commands.go`.
- Old `internal/cli/pull/run_pull.go` if no other command uses it after `run apply` owns diff application.

Remove or update tests and docs that assert the old surface:

- `internal/cli/app/run_start_test.go`.
- `internal/cli/app/run_logs_test.go`.
- `internal/cli/app/run_submit_test.go` old flag-shape cases.
- `internal/cli/app/run_status_test.go` should keep `run status --json` coverage and add `run status --follow` coverage.
- `internal/cli/job/log_test.go` default-format expectations.
- Server tests that assert `GET /v1/runs/{run_id}/logs`.
- `internal/cli/run/run_command_test.go` old help/command inventory expectations.
- `cmd/ploy/README.md` sections for old `ploy run --repo ...`, `run patch`, old `run pull`, `run start`, `run logs`, root submit `--json`, `--cap`, and `--max-retries`.
- Autocomplete files under `cmd/ploy/autocomplete/`.

Remove `GET /v1/runs/{run_id}/logs` with `ploy run logs`; `run status --follow` must not depend on it.

## Milestones

### 1. New CLI Shape

Scope:

- Update the Cobra command tree, help, and submit option model.
- Implement spec path resolution.
- Implement local/remote repo selector parsing.
- Remove old submit flags and command registrations.

Expected results:

- `ploy run <spec-path>` resolves CWD git `HEAD`.
- `ploy run <spec-path> <path>` resolves that local worktree.
- `ploy run <spec-path> namespace/repo` resolves remote ref `master`.
- Removed flags fail as unknown flags.

Testable outcome:

- Focused tests in `internal/cli/run`.
- App-level help tests updated.

### 2. Submit and Follow Semantics

Scope:

- Wire resolved repo inputs into run submission.
- Follow automatically only when `--apply` or `--pull` needs terminal success.
- Remove JSON submit output and cap/retry controls from the user surface.

Expected results:

- Submit prints run ID.
- `--pull` and `--apply` wait for success and fail on non-success terminal states.

Testable outcome:

- HTTP test captures the new submit request.
- Tests prove dirty local worktrees do not affect submit source selection.

### 3. Apply and Artifact Pull

Scope:

- Implement `ploy run apply`.
- Replace `ploy run pull` with artifact download.
- Delete `ploy run patch`.

Expected results:

- `run apply` applies accumulated diffs into a clean worktree whose `HEAD` matches the run source SHA, unless `--force` is used for SHA mismatch only.
- `run pull` writes artifacts and manifest to the selected or generated directory.
- `run patch` is gone.

Testable outcome:

- Fake-git tests for `git -C <path> apply`.
- HTTP artifact tests for default temp dir and explicit dir.

### 4. List, Docs, and Cleanup

Scope:

- Implement `run ls` selector while keeping `--limit` and `--offset`.
- Remove old pull/start/patch machinery and stale docs.
- Regenerate completions.

Expected results:

- Public docs match the final command set.
- No tests or docs reference removed flags or commands except historical design text.

Testable outcome:

- `go test ./internal/cli/... ./internal/server/handlers/...` passes.
- `go run tools/gencomp/main.go` produces expected autocomplete diffs.
- `~/@iw2rmb/amata/scripts/check_docs_links.sh` passes if docs under `docs/**` are changed.

## Acceptance Criteria

- `ploy run --help` shows only the new root syntax, `--apply`, `--pull`, and the retained subcommands.
- `ploy run <spec-path>` works from a git repo CWD and submits `HEAD`.
- `ploy run <spec-path>` outside a git repo hard fails before any HTTP submit.
- `ploy run <spec-path> <repo-path>` submits the selected repo path's `HEAD`, not the process CWD.
- `ploy run <spec-path> namespace/repo` submits remote ref `master`.
- `ploy run <spec-path> namespace/repo:branch` submits the selected branch.
- `ploy run <spec-path> namespace/repo:<sha>` submits the selected SHA.
- `ploy run <spec-path> <repo-path> --apply` applies only after a successful run and fails on dirty worktrees or source SHA mismatch.
- `ploy run <spec-path> --pull` downloads artifacts to a printed temp directory by default.
- `ploy run pull <run-id>` downloads artifacts and never performs git operations.
- `ploy run apply <run-id> <path>` applies accumulated diffs and never downloads artifact bundles.
- `ploy run apply <run-id> <path>` fails when local `HEAD` differs from run `source_commit_sha`.
- `ploy run apply <run-id> <path> --force` allows only the local `HEAD` mismatch.
- `ploy run apply` always fails when staged or unstaged diff exists, even with `--force`.
- `ploy run patch` is removed.
- `ploy run start` is removed.
- `ploy run logs` is removed.
- `GET /v1/runs/{run_id}/logs` is removed.
- Old root flags are removed.
- `ploy run status <run-id>` still renders status.
- `ploy run status --json <run-id>` still renders JSON status.
- `ploy run status <run-id> --follow` renders the live run status until the run reaches a terminal state.
- `ploy job log <job-id>` defaults to raw output.
- `ploy job log <job-id> --format structured` still renders structured output.

## Risks

- Current `POST /v1/runs` requires `target_ref`, even though the target command removes target branch as a user concept.
- Remote SHA submission may require a server contract that accepts commit SHA directly. The current server resolves a source SHA by running `git ls-remote <repo> <base_ref>`, which is branch/ref oriented.
- `run apply` needs repo resolution for a run. Existing `POST /v1/runs/{run_id}/pull` can resolve by repo URL, but its name and response are tied to old pull semantics.
- If `run ls` must filter by repo URL, the current API may need a small repo lookup or list-runs filter addition.

## References

- `design/next.md` contains the original TODO sketch for this command refactor.
- `internal/cli/app/root.go`
- `internal/cli/run/run_commands.go`
- `internal/cli/run/run_submit.go`
- `internal/cli/run/run_list.go`
- `internal/cli/run/run_artifact.go`
- `internal/cli/run/run_patch.go`
- `internal/cli/run/run_start.go`
- `internal/cli/pull/run_pull.go`
- `internal/cli/specpayload/mig_run_spec.go`
- `internal/domain/api/run_submit.go`
- `internal/domain/types/vcs.go`
- `internal/server/handlers/runs_submit.go`
- `internal/store/batchscheduler/batch_scheduler.go`
