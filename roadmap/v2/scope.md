# Roadmap v2 — Scope

## Repo selectors by URL (not id)

v2 extends the repo-scoped run surface to support repo selection by `repo_url` identity (e.g. `domain/owner/repo`) instead of `repo_id` (`mod_repos.id`).

Change entry: allow repo selectors by URL (human input), not internal ids.

- Current (HEAD): repo scoping uses `repo_id` (`mod_repos.id`) in `/v1/runs/{run_id}/repos/{repo_id}/...` and in CLI flags (see `roadmap/v1/cli.md`).
- Proposed (v2): allow repo selection by repo URL identity (`domain/owner/repo` or equivalent normalized form) for both API and CLI.
- Where: route parsing and lookup logic in `internal/server/handlers/*` for `/v1/runs/{run_id}/repos/{repo_selector}/...`, plus CLI flag parsing/normalization under `cmd/ploy/*` (use v0 normalization patterns from `cmd/ploy/mod_run_pull.go`).
- Compatibility: breaking if `repo_id` addressing is removed; additive if both selectors are accepted.
- Unchanged: underlying storage still keys repos by `mod_repos.id`; URL selectors only change how callers identify the repo.

Goal:

- Allow CLI and HTTP endpoints to use repo URL selectors without requiring users to look up internal IDs.

Changes (v2):

- CLI:
  - `ploy run diff --repo <repo-url>` (and any other repo-scoped commands) accept `domain/owner/repo` (or an equivalent normalized form).
  - Continue to support `--repo-id <mod_repo_id>` only if explicitly required by v2 scope (otherwise remove).
- API:
  - Repo-scoped endpoints under `/v1/runs/{run_id}/repos/...` must accept a repo URL selector (e.g. `domain/owner/repo`) in place of `repo_id`.
  - Server resolves the selector to the corresponding repo within the run.

Non-goals (v2):

- Backward compatibility guarantees between repo-id and repo-url addressing unless explicitly specified by v2 scope.
