#!/bin/zsh

set -euo pipefail
setopt null_glob

[[ -r ~/.zshenv ]] && source ~/.zshenv
[[ -r ~/.zshrc ]] && source ~/.zshrc

function log_info() {
  print -r -- "[ploy-v2] $*"
}

function abort() {
  log_info "ERROR: $*"
  exit 1
}

function ensure_cmd() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || abort "Required command '${cmd}' not found in PATH."
}

SCRIPT_PATH="$0"
if [[ "$SCRIPT_PATH" != /* ]]; then
  SCRIPT_PATH="$(pwd)/$SCRIPT_PATH"
fi
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DOCS_V2_DIR="$REPO_ROOT/docs/v2"
DOCS_DESIGN_DIR_DEFAULT="$REPO_ROOT/docs/design"
DOCS_DESIGN_DIR_ALT="$REPO_ROOT/docs/desing"
if [[ -d "$DOCS_DESIGN_DIR_ALT" && ! -d "$DOCS_DESIGN_DIR_DEFAULT" ]]; then
  DOCS_DESIGN_DIR="$DOCS_DESIGN_DIR_ALT"
  log_info "Using docs/desing directory for design outputs."
else
  DOCS_DESIGN_DIR="$DOCS_DESIGN_DIR_DEFAULT"
fi
DOCS_TASKS_DIR="$REPO_ROOT/docs/tasks"
DOCS_TASKS_BACKLOG_DIR="$DOCS_TASKS_DIR/v2"
TASK_QUEUE_PATH="$DOCS_TASKS_DIR/README.md"
DESIGN_SENTINEL="$DOCS_DESIGN_DIR/.ploy_v2_design_generated"

ensure_cmd codex
ensure_cmd git

mkdir -p "$DOCS_DESIGN_DIR" "$DOCS_TASKS_DIR" "$DOCS_TASKS_BACKLOG_DIR"

function run_codex() {
  local label="$1"
  shift
  log_info "$label"
  codex exec --dangerously-bypass-approvals-and-sandbox --include-plan-tool -C "$REPO_ROOT" - "$@"
}

function generate_design_docs() {
  if [[ -f "$DESIGN_SENTINEL" ]]; then
    log_info "Design sentinel present; skipping initial design synthesis."
    return
  fi

  log_info "First run detected; generating design documents from specs."

  local spec_path
  for spec_path in "$DOCS_V2_DIR"/*.md; do
    [[ -f "$spec_path" ]] || continue

    local spec_rel="${spec_path#$REPO_ROOT/}"
    local base_name="${spec_path:t:r}"
    local component_name
    if [[ "$base_name" == "README" ]]; then
      component_name="overview"
    else
      component_name="$base_name"
    fi

    local design_dir="$DOCS_DESIGN_DIR/$component_name"
    local design_md="$design_dir/README.md"
    mkdir -p "$design_dir"

    if [[ -s "$design_md" ]]; then
      log_info "Design doc ${design_md#$REPO_ROOT/} already exists; skipping generation."
      continue
    fi

    local design_rel="${design_md#$REPO_ROOT/}"
    run_codex "Generating design doc ${design_rel}" <<EOF
Follow the workflow guidance in docs/AGENTS.md. Fully review the specification at ${spec_rel}. Create a comprehensive design document at ${design_rel} that translates the spec into architecture decisions, workflows, data models, external integrations, invariants, risk mitigation, rollout plan, and testing strategy. Keep the narrative actionable for future implementation slices, reference related specs, and list open questions plus follow-up tasks. Make no code changes; only author ${design_rel} (adding any supporting assets beneath ${design_rel:h} if absolutely required).
EOF
  done

  touch "$DESIGN_SENTINEL"
  log_info "Design generation step complete."
}

function queue_tasks_from_design() {
  if [[ ! -d "$DOCS_DESIGN_DIR" ]]; then
    log_info "Design directory ${DOCS_DESIGN_DIR#$REPO_ROOT/} not found; skipping task planning."
    return
  fi

  log_info "Refreshing task backlogs from design documents."

  find "$DOCS_DESIGN_DIR" -type f -name '*.md' -print0 | sort -z | while IFS= read -r -d '' design_md; do
    local design_rel="${design_md#$REPO_ROOT/}"
    local relative_from_design="${design_md#$DOCS_DESIGN_DIR/}"
    local task_stub="${relative_from_design%.md}"
    if [[ "$task_stub" == */README ]]; then
      task_stub="${task_stub%/README}"
    fi
    if [[ -z "$task_stub" ]]; then
      task_stub="index"
    fi

    local tasks_md="$DOCS_TASKS_BACKLOG_DIR/$task_stub.md"
    local tasks_rel="${tasks_md#$REPO_ROOT/}"
    mkdir -p "$(dirname "$tasks_md")"

    run_codex "Planning tasks for ${design_rel}" <<EOF
Adhere to docs/AGENTS.md. Review the design narrative at ${design_rel} and ensure the implementation backlog at ${tasks_rel} exists and is current. Structure the Markdown with clear sections and checklists that respect the RED -> GREEN -> REFACTOR cadence, list prerequisite migrations/tooling, call out required tests (unit, CLI, integration mocks), and document doc updates or env vars from docs/envs/README.md. Preserve completed work, update or prune obsolete tasks, and keep outstanding items unchecked.

Once ${tasks_rel} is synchronised, update docs/tasks/README.md so the queue contains a single "- [ ] \`${tasks_rel}\`" entry (no duplicates, no in-progress markers unless you are actively executing the slice). Do not implement code or run tests; limit changes to backlog curation and queue bookkeeping.
EOF
  done
}

function remove_task_from_queue() {
  local task_rel="$1"
  [[ -f "$TASK_QUEUE_PATH" ]] || return
  python3 - "$task_rel" <<PYTHON
import pathlib, re, sys

queue_path = pathlib.Path(r"$TASK_QUEUE_PATH")
task = sys.argv[1]

lines = queue_path.read_text().splitlines()
pattern = re.compile(rf"^- \[[ xX]\] `{re.escape(task)}`$")
filtered = [line for line in lines if not pattern.match(line.strip())]

if filtered:
    queue_path.write_text("\n".join(filtered) + "\n")
else:
    queue_path.write_text("")
PYTHON
}

function open_tasks_from_queue() {
  [[ -f "$TASK_QUEUE_PATH" ]] || return
  awk '/^- \[ \] `/{gsub(/^- \[ \] `/, "", $0); gsub(/`$/, "", $0); print}' "$TASK_QUEUE_PATH"
}

function implement_open_tasks() {
  if [[ ! -f "$TASK_QUEUE_PATH" ]]; then
    log_info "Queue file ${TASK_QUEUE_PATH#$REPO_ROOT/} missing; skipping implementation."
    return
  fi

  log_info "Processing queued implementation slices."

  while true; do
    local -a pending_tasks
    IFS=$'\n' pending_tasks=($(open_tasks_from_queue || true))
    IFS=$' \t\n'

    if (( ${#pending_tasks[@]} == 0 )); then
      log_info "No pending tasks remain in docs/tasks/README.md."
      break
    fi

    local task_rel
    for task_rel in "${pending_tasks[@]}"; do
      [[ -n "$task_rel" ]] || continue

      if ! grep -Fq "- [ ] \`$task_rel\`" "$TASK_QUEUE_PATH"; then
        continue
      fi

      local task_md="$REPO_ROOT/$task_rel"
      if [[ ! -f "$task_md" ]]; then
        log_info "Task file ${task_rel} not found; removing stale queue entry."
        remove_task_from_queue "$task_rel"
        continue
      fi

      local component_name="${task_rel##*/}"
      component_name="${component_name%.md}"
      local commit_message="Implement ${component_name} tasks (Ploy v2)"

      run_codex "Implementing ${task_rel}" <<EOF
Obey docs/AGENTS.md. Fully implement every unchecked item in ${task_rel}. While working, mark the corresponding entry in docs/tasks/README.md as in-progress, follow the RED -> GREEN -> REFACTOR cadence, keep docs updated (including docs/envs/README.md when env vars shift), maintain >=60% overall test coverage and >=90% on critical workflow runner packages, and run relevant local tests (e.g. make test) before committing.

Once the plan in ${task_rel} is satisfied, mark all tasks complete, remove the queue entry for \`${task_rel}\`, and create a single git commit with message "${commit_message}" summarising the slice. The commit must include documentation updates and regenerated artifacts as required. Do not leave the repository dirty or the queue in an indeterminate state.
EOF

      if grep -Fq "\`${task_rel}\`" "$TASK_QUEUE_PATH"; then
        abort "Implementation for ${task_rel} left the queue entry in place; resolve manually."
      fi
    done
  done
}

generate_design_docs
queue_tasks_from_design
implement_open_tasks

log_info "Ploy v2 automation flow completed."
