# Stream 1 / Phase 1 — MVP Transflow: OpenRewrite + Build Check (Implemented)

Goal: Run an OpenRewrite recipe against a repo and verify the code builds (no deploy). Ship a runnable CLI first. Status: Implemented.

Reuse First
- Transform: `internal/arf` engine + recipes (services/openrewrite-jvm, internal/cli/arf).
- Build: `internal/cli/common/deploy.go::SharedPush` → API `/v1/apps/:app/builds` handled by `internal/build/trigger.go`.
- Git: `api/arf/git_operations.go` (clone/diff/commit) — extend with push and branch helpers.

Scope
- CLI entry: `ploy mod run -f transflow.yaml`.
- YAML: `roadmap/transflow/transflow.yaml` with global `lane` and `build_timeout` (default 10m), steps: `recipe`, `build`.
- Branching: create `workflow/<id>/<timestamp>`; commit after each step.
- Build-only check: generate app name `tfw-<id>-<timestamp>`; POST tar to `/v1/apps/:app/builds?env=dev[&lane=...]` and honor `build_timeout` (client-side).

Detailed Steps (small, verifiable)
- CLI entrypoint: add `transflow` command in `cmd/ploy/main.go` → `internal/mods/run.go`.
- YAML parsing: `internal/mods/config.go` → struct with `id`, `target_repo`, `base_ref`, `target_branch`, `lane`, `build_timeout`, `steps`.
- Git operations:
  - Use `CloneRepository` to clone `target_repo@base_ref`.
  - Add `CreateBranchAndCheckout(repoPath, branch)` and `PushBranch(remote, branch)` to `api/arf/git_operations.go`.
  - Use `CommitChanges` after recipe/build steps.
- Recipe step: call ARF transform via local CLI invocation (existing command) or API call; capture patch and commit.
- Build step:
  - Tar working tree (reuse `internal/cli/utils.TarDir` with `.gitignore`).
  - Call `SharedPush` with lane override from YAML if set; change `SharedPush` signature to accept a `timeout time.Duration` and honor `build_timeout`.
  - Fail step on non-200; surface response JSON in logs.
- Push branch to origin at the end (no MR yet).

Acceptance Criteria
- Given a repo + recipe, the runner applies recipe, commits to `workflow/<id>/<timestamp>`, build returns 200 with `lane` and `image|dockerImage`.

Post‑implementation notes (for Phase 2 compatibility)
- Ensure build error capture (stdout/stderr) is persisted to feed the LangGraph planner job.
- Persist a compact run manifest (repo metadata, lane, timing) for planner inputs.

TDD Plan
1) RED: unit tests for YAML parsing and app name/branch generation; fake `SharedPush` client; verify call shape.
2) GREEN: implement CLI flow and minimal glue; integrate ARF recipe apply; add timeout support.
3) REFACTOR: pare down duplication with existing CLI utilities.
