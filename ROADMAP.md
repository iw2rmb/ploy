# ploy mod run pull

Scope: Introduce a `ploy mod run pull` subcommand that replays Mods diffs into the current git worktree for a specific run/repository pair. The command resolves `<run-name|run-id>` plus the local git remote (default `origin`), enforces a clean working tree, normalizes the origin URL, verifies the run’s base commit is reachable from the remote, creates a new target branch, and applies the stored Mods diffs. A `--dry-run` flag validates and prints the planned operations without mutating the repository.

Documentation: ../auto/ROADMAP.md, cmd/ploy/README.md § "Batched Mod Runs", docs/mods-lifecycle.md, docs/how-to/create-mr.md, docs/how-to/deploy-a-cluster.md, docs/envs/README.md, docs/api/OpenAPI.yaml, docs/api/components/schemas/controlplane.yaml, docs/api/paths/mods_id.yaml, docs/api/paths/mods_id_diffs.yaml, docs/api/paths/diffs_id.yaml, internal/worker/hydration/git_fetcher.go, internal/nodeagent/execution.go.

Legend: [ ] todo, [x] done.

## CLI Command Surface
- [x] Add `ploy mod run pull` routing and flags — Expose the new subcommand and argument shape in the CLI entrypoint so users can invoke `ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>` from within a git repository.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (mods control-plane commands, mod run router)
  - Scope: 
    - Extend `handleModRun` in `cmd/ploy/mod_run.go` to dispatch `args[0] == "pull"` to a new `handleModRunPull` helper instead of falling through to `executeModRun`.
    - Implement `handleModRunPull(args []string, stderr io.Writer) error` in a new file `cmd/ploy/mod_run_pull.go` to keep pull-specific logic isolated from batch/list/repo handlers.
    - Define argument order as `ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>`; treat a final non-flag argument as `<run-name|run-id>` and default `origin` to `"origin"` when `--origin` is omitted.
    - On parse errors or missing `<run-name|run-id>`, print a focused usage line, e.g. `Usage: ploy mod run pull [--origin <remote>] [--dry-run] <run-name|run-id>`, mirroring existing `printModRun*Usage` helpers.
  - Snippets:
    - Router extension in `cmd/ploy/mod_run.go`:
      - `case "pull": return handleModRunPull(args[1:], stderr)`
    - Flag parsing sketch in `cmd/ploy/mod_run_pull.go`:
      - `fs := flag.NewFlagSet("mod run pull", flag.ContinueOnError); origin := fs.String("origin", "origin", "git remote to match (default origin)"); dryRun := fs.Bool("dry-run", false, "validate and print actions without mutating the repo")`
  - Tests: 
    - Add table-driven tests in `cmd/ploy/mod_run_batch_test.go` or a new `cmd/ploy/mod_run_pull_test.go` to verify:
      - Routing: `handleModRun([]string{"pull", "r1"}, ...)` calls `handleModRunPull`.
      - Usage errors: missing `<run-name|run-id>` and invalid flag combinations return the expected error strings and usage output.

## Origin Resolution & Working Tree Safety
- [x] Enforce git worktree detection, clean state, and normalized origin URL — Ensure `mod run pull` runs only inside a git repository with a clean working tree and a resolvable remote; derive a normalized origin URL compatible with server-side repo identification.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (mod_run_pull helper), internal/worker/hydration (reference for normalization)
  - Scope:
    - In `handleModRunPull`, detect that the current directory is inside a git worktree:
      - Execute `git rev-parse --is-inside-work-tree` using the same `exec.CommandContext` pattern as `internal/worker/hydration/git_fetcher.go::runGitCommand`, with `GIT_TERMINAL_PROMPT=0` and `GIT_ASKPASS=echo` to avoid interactive prompts.
      - If not inside a worktree, return an error like `mod run pull: must be run inside a git repository`.
    - Require a clean working tree (no staged or unstaged changes) before making any modifications:
      - Run `git status --porcelain=v1` in the repo root; if the output is non-empty, print a concise error: `mod run pull: working tree must be clean (commit or stash changes first)` and abort.
    - Resolve the requested remote (default `"origin"` or the value of `--origin`):
      - Call `git remote get-url <origin>` and capture `stdout`.
      - If the remote does not exist, error: `mod run pull: git remote "<origin>" not found`.
    - Normalize the origin URL using the same semantics as `internal/worker/hydration/git_fetcher.go::normalizeRepoURL`:
      - Implement a small helper in the CLI (e.g., `normalizeRepoURLForCLI`) that trims whitespace, removes a trailing slash, and strips a trailing `.git` suffix from the remote URL string.
      - Keep the raw remote URL string for exact equality where required (e.g., when calling `/v1/repos/{repo_id}/runs`), and use the normalized form only for comparison / matching.
  - Snippets:
    - Normalization reference from `internal/worker/hydration/git_fetcher.go`:
      - `normalized := strings.TrimSpace(raw); normalized = strings.TrimSuffix(normalized, "/"); normalized = strings.TrimSuffix(normalized, ".git")`
    - Clean-WT check:
      - `git status --porcelain=v1` → treat any non-empty output as “dirty”.
  - Tests:
    - Unit tests in `cmd/ploy/mod_run_pull_test.go` using a temporary git repository:
      - Dirty working tree (untracked or modified files) triggers a clear error and no further actions.
      - Missing remote `origin` or a custom `--origin` fails with the expected error.
      - Origin URL normalization removes `.git` and trailing slashes but leaves scheme and host intact.

## Run & Repo Resolution
- [x] Resolve `<run-name|run-id>` plus origin repo to a unique Mods run/repo combination — Use the repo-centric API to locate the correct run for the current repository, honoring both UUIDs and human-readable run names while selecting the first matching result.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (mod_run_pull), internal/server/handlers/repos.go, internal/store/queries/run_repos.sql, docs/api
  - Scope:
    - Derive the repo identifier for the `/v1/repos/{repo_id}/runs` API:
      - Use the raw origin URL string from `git remote get-url` as `repo_id` (URL-encoded for the path segment) so it matches the stored `run_repos.repo_url` value populated via `CreateRunRepoParams.RepoUrl`.
      - If needed in future, consider documenting in `docs/envs/README.md` that `--repo-url` should match the git remote reported by `git remote get-url <origin>` to enable `mod run pull`.
    - Call `GET /v1/repos/{repo_id}/runs?limit=N&offset=0` (start with `limit=100` to comfortably cover recent history), using the HTTP client from `resolveControlPlaneHTTP`.
      - Reuse the response type `RepoRunSummary` from `internal/server/handlers/repos.go` on the CLI side (define a mirrored struct under `internal/cli/mods` or a new package to avoid import cycles).
    - Implement run resolution rules:
      - Treat the final positional argument as `<run-name|run-id>`.
      - First, try to match `RepoRunSummary.RunID` by string equality (IDs are KSUID-backed strings but treated as opaque by the CLI).
      - If no `RunID` match is found, match against `RepoRunSummary.Name` (after trimming spaces).
      - Filter only runs whose `RepoStatus` indicates that execution actually ran (e.g., `succeeded`, `failed`, or `skipped`), not purely pending repos; this ensures diffs exist or meaningful errors are returned.
      - If multiple results match the same name, **do not** error; select the first entry returned by the API, which is ordered by `run_repos.created_at DESC` per `ListRunsForRepo` (satisfies the “do not guard multiple run-name results (just select first)” requirement).
      - If no matching run is found for the given `<run-name|run-id>` and origin, return: `mod run pull: no run found for <run-name|run-id> and origin <origin>`.
    - Capture from the selected `RepoRunSummary`:
      - Parent `run_id` (batch or single run) for diagnostics.
      - `base_ref`, `target_ref`, and `attempt` for branch naming and logging.
    - To support diff/commit lookup for the specific repo execution, extend the repo-centric API to surface the child execution run id associated with this `run_repos` row:
      - Add `execution_run_id` (string, nullable) to `RepoRunSummary` in `internal/server/handlers/repos.go` and wire it from `run_repos.execution_run_id` (TEXT KSUID-backed column).
      - Update `ListRunsForRepo` query in `internal/store/queries/run_repos.sql` (and generated `run_repos.sql.go`) to select `rr.execution_run_id` and alias it appropriately.
      - Document the new field in `docs/api/components/schemas/controlplane.yaml#/RepoRunSummary` and ensure `docs/api/OpenAPI.yaml` references remain correct.
  - Snippets:
    - Existing `RepoRunSummary` in `internal/server/handlers/repos.go`:
      - `type RepoRunSummary struct { RunID string \`json:"run_id"\`; Name *string \`json:"name,omitempty"\`; ... }`
    - New field sketch (KSUID-backed string ID):
      - `ExecutionRunID *string \`json:"execution_run_id,omitempty"\`` populated from `rr.execution_run_id` when non-null.
  - Tests:
    - Extend `internal/server/handlers/repos_test.go` to:
      - Verify that `GET /v1/repos/{repo_id}/runs` includes `execution_run_id` when `run_repos.execution_run_id` is set.
    - Add CLI tests in `cmd/ploy/mod_run_pull_test.go`:
      - Use an httptest server that serves a canned `/v1/repos/{repo_id}/runs` response with multiple runs and confirm that:
        - When `<run-name|run-id>` equals a `RunID` value, that entry is selected (ID selectors prefer `RunID`).
        - When `<run-name|run-id>` does not match any `RunID` but matches `Name`, the first matching `Name` is selected when multiple entries share the name.

## Diff Retrieval, Branch Creation, and Patch Application
- [x] Fetch commit SHA and diffs for the resolved run, create a new branch, and apply patches — Use the execution run id to locate the Mods run, verify the base commit exists on the origin remote, create the `target-ref` branch at that commit, and apply all stored diffs via `git apply`. Support `--dry-run` to perform all lookups and validations without mutating the git repository.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy (mod_run_pull), internal/cli/mods (diffs and inspect clients), internal/server/handlers/mods_ticket.go, internal/nodeagent/execution.go, docs/api
  - Scope:
    - Resolve the execution Mods run:
      - Use `RepoRunSummary.ExecutionRunID` (string) as the Mods run id for status and diff queries.
      - If `ExecutionRunID` is nil or empty, return a clear error: `mod run pull: execution run id missing for <run-name|run-id> (repo may not have started)`.
    - Fetch run status to obtain base ref, target ref, and commit SHA:
      - Call `GET /v1/mods/{id}` where `{id}` is the execution run id.
      - Reuse `modsapi.TicketStatusResponse` decoding helpers from `cmd/ploy/mod_run_artifact.go` or `internal/cli/mods/inspect.go`, and surface:
        - `repo_url` (for diagnostics; should match origin).
        - `base_ref`, `target_ref`, and `commit_sha` from `RunStatus` (see `docs/api/components/schemas/controlplane.yaml#/RunStatus`).
      - If `commit_sha` is empty, treat this as a hard error for `mod run pull` and prompt the user to rerun the Mods flow with commit pinning: `mod run pull: commit_sha is not available for this run; pull requires a pinned commit`.
    - Verify commit SHA reachability on the origin remote:
      - Run `git fetch <origin> <commit_sha> --depth=1` in the current repo using the same `runGitCommand` pattern (non-interactive).
      - If fetch fails with “couldn't find remote ref” or similar, return: `mod run pull: commit <sha> not reachable from origin "<origin>" (force-push or mirror mismatch)` and abort without creating any branches.
    - Create the target branch from the commit SHA, using fetch + branch instead of cloning:
      - Determine the branch name:
        - Prefer the per-repo `target_ref` from `RepoRunSummary.TargetRef` when present; otherwise, fall back to the execution run’s `target_ref` from `RunStatus`.
      - Before creating the branch, check both local and remote collisions:
        - `git show-ref --verify refs/heads/<target_ref>`; if it exists, fail with `mod run pull: branch "<target_ref>" already exists locally`.
        - `git ls-remote --heads <origin> <target_ref>`; if it exists, fail with `mod run pull: branch "<target_ref>" already exists on remote "<origin>"`.
      - Create the branch and switch to it:
        - `git branch <target_ref> <commit_sha>`
        - `git checkout <target_ref>`
      - For `--dry-run`, **do not** execute the `git branch` or `git checkout` calls; instead, print a summary to stderr:
        - `Would create branch "<target_ref>" at <commit_sha> (origin "<origin>") and apply Mods diffs`.
    - Download and apply Mods diffs:
      - Use the existing Mods diffs API:
        - List diffs: `GET /v1/mods/{execution_run_id}/diffs`.
        - For each diff id, download patch: `GET /v1/diffs/{diff_id}?download=true` (returns gzipped bytes).
      - Prefer reusing or extending `internal/cli/mods.DiffsCommand` so that `mod run pull` can obtain **all** diffs for the run, not just the newest:
        - Option A (minimal): add a helper (e.g., `ListAllDiffs`) under `internal/cli/mods` that mirrors the first part of `DiffsCommand.Run` but returns the full `diffs` array instead of printing or downloading a single patch.
        - Option B: introduce a new command object (e.g., `DiffsListCommand`) that returns the list of diff IDs and metadata.
      - For each diff:
        - Decompress gzipped patch bytes using the same logic as `internal/nodeagent/execution.go::decompressPatch`.
        - Skip empty patches (after trimming whitespace), matching `applyGzippedPatch` semantics.
        - Apply the patch via `git apply` in the current worktree (no `--index`), using the same error reporting semantics as `applyGzippedPatch` (`git apply failed: <stderr>`).
      - For `--dry-run`, do not call `git apply`; instead, print the number of diffs and their IDs, plus a summary of the approximate patch size, e.g.:
        - `Would apply 3 Mods diffs for run <execution_run_id> onto branch "<target_ref>"`.
    - On successful non-dry-run completion:
      - Print a concise success message to stderr: `Applied N Mods diffs from run <run-id> to branch "<target_ref>" (origin "<origin>")`.
  - Snippets:
    - Commit fetch pattern from `internal/worker/hydration/git_fetcher.go::cloneAndCheckout`:
      - `git fetch origin <commit_sha> --depth 1`
    - Patch application reference from `internal/nodeagent/execution.go::applyGzippedPatch`:
      - `cmd := exec.CommandContext(ctx, "git", "apply")` with `cmd.Dir = workspace`.
  - Tests:
    - Unit tests in `cmd/ploy/mod_run_pull_test.go` using a fake HTTP server and a temporary git repo:
      - Happy path: reachable `commit_sha`, no existing branch, and non-empty diffs → branch created and `git apply` invoked; verify final HEAD matches expected commit and files are patched (simple file content assertion).
      - Existing local or remote branch with the same `target_ref` short-circuits with appropriate error, and no `git apply` is performed.
      - Unreachable `commit_sha` (simulated by failing `git fetch`) causes a clear error and no branch or patch application.
      - `--dry-run` performs all HTTP and git validation steps but leaves the working tree and branch list unchanged.

## Documentation & OpenAPI Updates
- [x] Document `ploy mod run pull` behavior and keep API schema aligned — Update user-facing docs and OpenAPI definitions to describe the new workflow and any added fields used by the CLI.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs (cmd/ploy/README.md, docs/mods-lifecycle.md, docs/how-to), docs/api
  - Scope:
    - Extend `cmd/ploy/README.md`:
      - Add `mod run pull` to the command summary table and include a short usage example:
        - From a repo that participated in a Mods batch: `ploy mod run pull java17-fleet` (reconstructs the Mods branch for the current origin).
      - Clarify that `<run-name|run-id>` may be either the run ID (KSUID string) or the unique batch name passed via `--name`, and that `run-name|run-id + origin` selects the corresponding repo within that run.
    - Update `docs/mods-lifecycle.md`:
      - Add a subsection under the Mods workflow explaining how diffs stored in `diffs` are now consumable by the CLI via `mod run pull`, including a high-level sequence:
        - Resolve run + repo, fetch `commit_sha`, create branch, apply diffs.
      - Reference the normalization and clean working tree requirements for safety.
    - Update `docs/how-to/create-mr.md` and `docs/how-to/deploy-a-cluster.md`:
      - Add a short “pull locally” follow-up example after batch creation, e.g.:
        - `cd service-a && ploy mod run pull java17-fleet`.
    - Ensure OpenAPI docs match any API changes:
      - If `RepoRunSummary` gains `execution_run_id`, update `docs/api/components/schemas/controlplane.yaml#/RepoRunSummary` and regenerate or adjust `docs/api/OpenAPI.yaml` references.
  - Snippets:
    - Example doc snippet for README:
      - ```bash
        # Reconstruct Mods changes for the current repo
        ploy mod run pull java17-fleet
        ```
  - Tests:
    - Run `go test ./docs/api/...` or the existing OpenAPI verification test (`docs/api/verify_openapi_test.go`) to confirm schema changes are wired correctly.
    - Run `make test` to ensure new CLI tests and existing guardrails pass after documentation and schema updates.
