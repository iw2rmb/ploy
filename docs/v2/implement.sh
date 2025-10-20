 #!/bin/zsh
#
# Orchestrates a full Ploy v2 implementation cycle using the Codex CLI in
# non-interactive mode. The script follows four phases:
#   1. Generate design documents for every major v2 concern.
#   2. Expand those designs into actionable tasks and update docs/tasks/README.md.
#   3. Iterate through each queued task, implementing it and creating a commit.
#   4. After each task, prune its design/task artefacts and fan out feature specs
#      (docs/features/<feature>/README.md, docs/envs/README.md, docs/api/OpenAPI.yaml, etc.).
#
# Environment variables:
#   CODEX_BIN   Path to the Codex CLI (defaults to `codex`).
#   GIT_BIN     Path to git (defaults to `git`).

set -euo pipefail

export PATH="/opt/homebrew/opt/coreutils/libexec/gnubin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
hash -r

CODEX_BIN=${CODEX_BIN:-$(command -v codex 2>/dev/null || echo codex)}
GIT_BIN=${GIT_BIN:-$(command -v git 2>/dev/null || echo git)}

function run_codex() {
  local prompt="$1"
  "$CODEX_BIN" exec \
    --dangerously-bypass-approvals-and-sandbox \
    -- "${prompt}"
}

function generate_design_docs() {
  local designs=(
    "runtime-queue::docs/design/v2/runtime-and-queue.md::Document the etcd queue, job lifecycle, capacity reporting, and retry semantics. Reference docs/v2/job.md, docs/v2/queue.md, docs/v2/etcd.md, and outline API surfaces."
    "build-gate::docs/design/v2/build-gate-integration.md::Describe how SHIFT integrates without Grid, referencing docs/v2/shift.md. Include workflow between ploynode and SHIFT."
    "artifacts-logging::docs/design/v2/artifacts-and-logging.md::Detail IPFS usage, log archival, GC lifecycle (docs/v2/ipfs.md, docs/v2/logs.md, docs/v2/gc.md)."
    "ops-cli::docs/design/v2/ops-and-cli.md::Clarify CLI commands, deployment flows, version tag handling, testing requirements (docs/v2/cli.md, docs/v2/devops.md, docs/v2/testing.md)."
  )

  for entry in "${designs[@]}"; do
    IFS='::' read -r slug path description <<<"$entry"
    run_codex """
Consult and comply with ~/.codex/AGENTS.md.
Create or update ${path} covering: ${description}.
Document assumptions, open questions, and acceptance criteria. Keep Markdown lint compliant.
Use web search to gather latest library versions, best practices, and relevant implementation snippets.
"""
  done
}

function build_task_queue() {
  run_codex """
Follow ~/.codex/AGENTS.md.
Read all v2 design docs under docs/design/v2 and produce a dependency-ordered task list.
Update docs/tasks/README.md (using "- [ ] path/to/task.md") and create placeholder specs under docs/tasks/ for each entry.
Reference docs/v2/testing.md wherever additional guidance is needed.
Use web search for best practices or external standards while planning tasks.
"""
}

function read_tasks() {
  rg '^\- \[ \] ' docs/tasks/README.md | sed 's/^\- \[ \] //' | while read -r task; do
    print -r -- "$task"
  done
}

function implement_tasks() {
  local task
  while task=$(read_tasks | head -n1); [[ -n "$task" ]]; do
    local task_file="docs/tasks/${task}"
    local feature_dir="docs/features/${task:h}"
    local feature_spec="${feature_dir}/README.md"

    run_codex """
Adhere to ~/.codex/AGENTS.md.
Implement the task described in ${task_file}:
1. Update source code to satisfy the task spec and associated design doc.
2. Add/adjust tests (unit and integration) with explicit timeouts (see docs/v2/testing.md).
3. Update related specs (docs/v2/README.md, docs/v2/queue.md, docs/v2/job.md, docs/v2/ipfs.md, docs/v2/logs.md, docs/v2/etcd.md, docs/v2/testing.md) if behaviour changes.
4. Maintain docs/features structure: ensure ${feature_spec} reflects current behaviour/spec.
5. Remove the design doc if all its tasks are complete.
6. Run gofmt/go test and other project linters.
7. Stage changes, commit with message "feat: implement ${task}".
8. Remove ${task_file} and drop the entry from docs/tasks/README.md.
Use web search for latest versions, best practices, and relevant snippets.
"""

    "$GIT_BIN" status --short
  done
}

function main() {
  generate_design_docs
  build_task_queue
  implement_tasks
  run_codex """
Perform a final verification pass in line with ~/.codex/AGENTS.md:
- Ensure docs/envs/README.md and docs/api/OpenAPI.yaml reflect the final state.
- Run make lint-md and go test ./...
- Provide a summary of the completed migration.
- Use web search to confirm no newer versions or practices were missed.
"""
}

main "$@"
