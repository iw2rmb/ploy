# Task Queue (Ordered by Blocking)

Follow the reservation protocol in `AGENTS.md`: reopen this file, flip a task to
`[x]` when you are actively working it, and remove the line once the change lands.
Unblocked work appears first; blocked tasks reference their prerequisites.

- [x] docs/tasks/mods-grid/01-red.md — RED tests for Mods repository materialisation & job specs (unblocked)
- [x] docs/tasks/mods-grid/02-green.md — Implement repo materialisation + Mods job specs (blocked by 01-red)
- [x] docs/tasks/mods-grid/03-red.md — RED tests for build-gate feedback & healing branches (blocked by 02-green)
- [x] docs/tasks/mods-grid/04-green.md — Implement healing retries & parallel options (blocked by 03-red)
- [ ] docs/tasks/mods-grid/05-refactor.md — Migrate Mods lanes into the shared lane catalog (blocked by 04-green)
- [ ] docs/tasks/mods/orw-test.md — Restore OpenRewrite scenario playbooks (unblocked; legacy follow-up)
- [x] docs/tasks/roadmap/19-mods-parallel-planner.md — Legacy planner slice (reopened for context, no active work)
- [x] docs/tasks/roadmap/21-build-gate-reboot.md — Build gate reboot complete (reference only)
- [x] docs/tasks/workflow-rpc-alignment/04-helper-adoption.md — Helper adoption complete (reference only)
- [x] docs/tasks/build-gate/09-eslint-adapter.md — ESLint adapter complete (reference only)
- [x] docs/tasks/knowledge-base/03-cli-evaluate.md — KB CLI evaluate complete (reference only)
- [x] docs/tasks/discovery-alignment/03-workflow-grid-alignment.md — Discovery alignment complete (reference only)
- [x] docs/tasks/integration-manifests/01-schema-upgrade.md — Integration manifest schema v2 complete (reference only)
- [x] docs/tasks/nats/10-health-cleanup.md — NATS health cleanup complete (reference only)
- [x] docs/tasks/recipes/README.md — Recipes registry tracking (reference only)
- [x] docs/tasks/deploy/README.md — Legacy deploy plan (reference only)
- [x] docs/tasks/mod-report/README.md — Mod report status log (reference only)
