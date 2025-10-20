#!/bin/zsh
set -euo pipefail

export PATH="/opt/homebrew/opt/coreutils/libexec/gnubin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
hash -r
CODEX_BIN=${CODEX_BIN:-$(command -v codex 2>/dev/null || echo codex)}
GIT_BIN=${GIT_BIN:-$(command -v git 2>/dev/null || echo git)}

PLAN_DOC="docs/v2/implementation-plan.md"
FINAL_VERIFICATION_MARKER="docs/v2/.final-verification-done"
typeset -a DESIGN_SPECS=(
  "runtime-queue::docs/design/v2/runtime-and-queue.md::Document the etcd queue, job lifecycle, capacity reporting, and retry semantics. Reference docs/v2/job.md, docs/v2/queue.md, docs/v2/etcd.md, and outline API surfaces."
  "build-gate::docs/design/v2/build-gate-integration.md::Describe how SHIFT integrates without Grid, referencing docs/v2/shift.md. Include workflow between ploynode and SHIFT."
  "artifacts-logging::docs/design/v2/artifacts-and-logging.md::Detail IPFS usage, log archival, GC lifecycle (docs/v2/ipfs.md, docs/v2/logs.md, docs/v2/gc.md)."
  "ops-cli::docs/design/v2/ops-and-cli.md::Clarify CLI commands, deployment flows, version tag handling, testing requirements (docs/v2/cli.md, docs/v2/devops.md, docs/v2/testing.md)."
)
typeset -a DESIGN_TASK_MAPPINGS=(
  "docs/design/v2/runtime-and-queue.md::docs/tasks/ploy-v2-runtime-queue::Runtime queue implementation (transactional claims, job ledger, capacity heartbeats, headlessly driven monitoring)."
  "docs/design/v2/artifacts-and-logging.md::docs/tasks/ploy-v2-artifacts-logging::Artifact metadata, log archival, GC/audit."
  "docs/design/v2/build-gate-integration.md::docs/tasks/ploy-v2-build-gate::SHIFT library integration, ploynode execution path."
  "docs/design/v2/ops-and-cli.md::docs/tasks/ploy-v2-ops-cli::CLI command alignment, release/testing rigor."
)

function run_codex() {
  local prompt="$1"
  "$CODEX_BIN" exec --dangerously-bypass-approvals-and-sandbox -- "$prompt"
}

function plan_overall_steps() {
  if [[ -s "$PLAN_DOC" ]]; then
    return 1
  fi
  printf 'Planning execution steps in %s\n' "$PLAN_DOC"
  run_codex "$(cat <<'EOF'
Consult and comply with ~/.codex/AGENTS.md.
Create or update docs/v2/implementation-plan.md with a concise, dependency-aware series of independent steps for the Ploy v2 migration workflow executed by docs/v2/implement.sh.
Structure the plan as a Markdown checklist where every item triggers a fresh Codex session, while the script continues through the sequence in the same invocation (design creation, task planning, implementation, verification).
Call out prerequisites (environment variables from docs/envs/README.md) and cross-reference relevant design docs and specs.
No other actions are required in this session.
EOF
)"
  return 0
}

function generate_design_docs() {
  local entry slug path description
  local status=1
  for entry in "${DESIGN_SPECS[@]}"; do
    IFS='::' read -r slug path description <<<"$entry"
    if [[ -s "$path" ]]; then
      continue
    fi
    printf 'Generating design document %s\n' "$path"
    mkdir -p "$(dirname "$path")"
    run_codex "$(cat <<EOF
Consult and comply with ~/.codex/AGENTS.md.
Create or update ${path} covering: ${description}.
State explicitly that Ploy v2 does not need to maintain backward compatibility with legacy Grid behaviour; reuse existing Grid code only when helpful.
Document assumptions, open questions, acceptance criteria. Keep Markdown lint compliant.
Use web search to gather latest library versions, best practices, relevant snippets.
EOF
)"
    status=0
  done
  return $status
}

function ensure_task_readme_header() {
  local readme="docs/tasks/README.md"
  if [[ ! -f "$readme" ]]; then
    cat <<'EOF' >"$readme"
# Dependency-Ordered Task Queue

Always re-open this file immediately before editing; do not rely on a cached copy or stale editor buffer.

## Legend

- [ ] `relative/task/path.md` — task is planned and not yet claimed.
- [x] `relative/task/path.md` — task is currently being implemented; only one agent should hold this state.
- Remove the entry once the task is complete so downstream work can advance.

## Queue
EOF
    return
  fi
  if ! rg -q '^## Queue' "$readme"; then
    cat <<'EOF' >>"$readme"

## Queue
EOF
  fi
}

function read_tasks() {
  rg '^\- \[ \] ' docs/tasks/README.md 2>/dev/null | sed 's/^\- \[ \] //' || true
}

function build_task_queue() {
  ensure_task_readme_header
  local entry design prefix summary
  local status=1
  for entry in "${DESIGN_TASK_MAPPINGS[@]}"; do
    IFS='::' read -r design prefix summary <<<"$entry"
    if [[ ! -s "$design" ]]; then
      continue
    fi
    if rg -q -F "- [ ] ${prefix}/" docs/tasks/README.md 2>/dev/null || \
       rg -q -F "\`${prefix}" docs/tasks/README.md 2>/dev/null; then
      continue
    fi
    printf 'Planning task queue entries for %s\n' "$design"
    local queue_entries
    queue_entries=$(run_codex "$(cat <<EOF
Follow ~/.codex/AGENTS.md.
Read ${design} and break it into concise tasks (name, scope, acceptance criteria) under ${prefix}.
Output them as Markdown bullet points with the format '- [ ] path/to/task.md — summary', referencing docs/v2/testing.md for expectations.
Explicitly state "No legacy Grid compatibility required" in each summary if the design suggests reuse.
Use web search for best practices while planning tasks. Respond ONLY with the bullet list.
EOF
)")
    if [[ -z "${queue_entries// }" ]]; then
      printf 'Codex did not return task entries for %s; aborting.\n' "$design" >&2
      exit 1
    fi
    {
      printf '\n'
      printf '%s\n' "$queue_entries"
      printf '\n'
    } >>docs/tasks/README.md
    status=0
  done
  return $status
}

function implement_tasks() {
  local task path abs feature_spec
  local status=1
  while task=$(read_tasks | head -n1); [[ -n "${task// }" ]]; do
    path=${task%% — *}
    abs="docs/tasks/${path}"
    feature_spec="docs/features/${path%/*}/README.md"
    printf 'Implementing task %s\n' "$path"
    run_codex "$(cat <<EOF
Adhere to ~/.codex/AGENTS.md.
Implement the task ${abs} (new Ploy v2 behaviour—no legacy Grid compatibility).
Requirements:
1. Update code to satisfy the task spec and relevant design doc.
2. Add/adjust tests (unit & integration) with explicit timeouts; cover success/failure paths.
3. Update relevant specs (docs/v2/README.md, docs/v2/queue.md, docs/v2/job.md, docs/v2/ipfs.md, docs/v2/logs.md, docs/v2/etcd.md, docs/v2/testing.md).
4. Update ${feature_spec} with the final behaviour.
5. Run gofmt, go test ./..., and make lint-md before completing.
6. Stage changes, commit with message "feat: implement ${path}".
7. Remove ${abs} and drop the entry from docs/tasks/README.md.
Use web search for latest libraries, best practices, snippets.
EOF
)"
    run_codex "Perform a quick status: git status --short"
    status=0
  done
  return $status
}

function final_verification() {
  if [[ -f "$FINAL_VERIFICATION_MARKER" ]]; then
    return 1
  fi
  if [[ -n "$(read_tasks)" ]]; then
    return 1
  fi
  printf 'Running final verification via Codex\n'
  run_codex "$(cat <<'EOF'
Perform final verification (/.codex/AGENTS.md):
- Ensure docs/envs/README.md, docs/api/OpenAPI.yaml are updated.
- Run make lint-md and go test ./...
- Summarise the completed migration; note that no backward compatibility is required.
- Use web search to ensure nothing is outdated.
EOF
)"
  touch "$FINAL_VERIFICATION_MARKER"
  return 0
}

function main() {
  local did_work=1
  if plan_overall_steps; then
    printf 'Plan generated.\n'
    did_work=0
  fi
  if generate_design_docs; then
    printf 'Design documents generated.\n'
    did_work=0
  fi
  if build_task_queue; then
    printf 'Task queue updated.\n'
    did_work=0
  fi
  if implement_tasks; then
    printf 'Task implementation steps executed.\n'
    did_work=0
  fi
  if final_verification; then
    printf 'Final verification executed. Workflow complete.\n'
    did_work=0
  fi
  if [[ $did_work -ne 0 ]]; then
    printf 'No pending steps detected. Nothing to do.\n'
  fi
}

main "$@"
