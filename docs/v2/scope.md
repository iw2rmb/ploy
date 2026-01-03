# Roadmap v2 — Scope

## Repo selectors by URL (not id)

v2 extends the repo-scoped run surface to support repo selection by `repo_url` identity (e.g. `domain/owner/repo`) instead of `repo_id` (`mod_repos.id`).

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
