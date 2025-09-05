# Stream 3 · Phase 1 — Merge Requests (Provider: GitLab)

Goal: create/update an MR on GitLab for the workflow branch.

Reuse First
- Git ops: reuse `api/arf/git_operations.go` for clone/commit; extend with push.
- MR body: adapt existing MR description pattern from ARF CLI.

Scope
- Provider abstraction: `git_provider = gitlab` (default) with envs `GITLAB_URL`, `GITLAB_TOKEN` (no secrets in YAML).
- Infer GitLab project (namespace/name) from `target_repo` URL in transflow.yaml.
- MR target branch is always `base_ref`.
- Create workflow branch at start; push commits; create MR (title/body from step summaries); update MR on new commits.

Implementation Steps
- Add `internal/git/provider` with minimal `CreateOrUpdateMR` for GitLab: infer project from `target_repo` HTTPS.
- Extend CLI runner to push branch (HTTPS URL with token) and call provider to create/update MR.
- Include MR URL in final transflow summary.
- Set default MR labels/scope to `ploy` and `tfl` when GitLab instance supports labels; otherwise include them in MR description body.
 - Reuse `workflow/<id>/<timestamp>` branch name across runs to update the same MR.

Deliverables
- `mr` output field in transflow result with MR URL/ID.
- Minimal GitLab REST calls: create/update MR (project from `target_repo`, source branch, target `base_ref`, title, description).

Acceptance
- After successful build, branch is pushed and an MR is opened on GitLab; subsequent runs against same branch update the MR.

Out of scope
- Reviewers/labels automation; provider=github (Phase 2).

TDD Plan
1) RED: GitLab API mocks for create/update MR.
2) GREEN: implement minimal client and wiring from transflow runner.
3) REFACTOR: shared provider interface for Phase 2.
