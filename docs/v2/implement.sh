#!/bin/zsh

set -euo pipefail

source ~/.zshenv 2>/dev/null || true
source ~/.zshrc 2>/dev/null || true

log() {
  print -ru2 -- "[ploy-v2] $*"
}

SCRIPT_PATH=${(%):-%x}
SCRIPT_DIR=$(cd "$(dirname "$SCRIPT_PATH")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

SPEC_DIR="$REPO_ROOT/docs/v2"
DESIGN_DIR="$REPO_ROOT/docs/design"
TASKS_DIR="$REPO_ROOT/docs/tasks"
TASK_QUEUE="$TASKS_DIR/README.md"
TASK_QUEUE_REL="docs/tasks/README.md"
INITIATIVE_PREFIX="ploy-v2"

typeset -a FEATURES=()
typeset -A FEATURE_SPEC_MAP

for spec_file in "$SPEC_DIR"/*.md; do
  [[ -f "$spec_file" ]] || continue
  base_name=${spec_file:t:r}
  if [[ "$base_name" == "README" ]]; then
    feature_slug="overview"
  else
    feature_slug=${base_name:l}
  fi
  spec_rel=${spec_file#$REPO_ROOT/}
  FEATURES+=("$feature_slug")
  FEATURE_SPEC_MAP[$feature_slug]="$spec_rel"
done

if (( ${#FEATURES[@]} == 0 )); then
  log "No specs discovered in ${SPEC_DIR}; nothing to process."
  exit 0
fi

mkdir -p "$DESIGN_DIR" "$TASKS_DIR"

# run_codex re-sources the zsh environment and streams a prompt into codex exec.
run_codex() {
  local prompt="$1"
  local tmpfile
  tmpfile=$(mktemp)
  print -r -- "$prompt" > "$tmpfile"
  source ~/.zshenv 2>/dev/null || true
  source ~/.zshrc 2>/dev/null || true
  log "codex exec starting with prompt file ${tmpfile}"
  if codex --dangerously-bypass-approvals-and-sandbox --search exec --cd "$REPO_ROOT" - < "$tmpfile"; then
    rm -f "$tmpfile"
    log "codex exec completed successfully"
  else
    log "codex exec failed; preserving prompt at ${tmpfile}"
    return 1
  fi
}

# generate_design_docs asks Codex to transform each v2 spec into a design doc.
generate_design_docs() {
  log "Starting design generation for ${#FEATURES[@]} features"
  for feature in "${FEATURES[@]}"; do
    spec_rel=${FEATURE_SPEC_MAP[$feature]}
    design_dir_rel="docs/design/${feature}"
    design_doc_rel="${design_dir_rel}/README.md"
    mkdir -p "$REPO_ROOT/$design_dir_rel"
    log "Generating design doc ${design_doc_rel} from ${spec_rel}"
    prompt=$(cat <<EOF
You are Codex operating inside the Ploy repository. Follow /Users/vk/@iw2rmb/docs/AGENTS.md and docs/AGENTS guidance verbatim.

Objective: Convert the v2 spec at ${spec_rel} into a design document stored at ${design_doc_rel}.

Constraints:
- Use /Users/vk/@iw2rmb/docs/templates/design/README.md as the scaffold.
- Use identifier prefix ${INITIATIVE_PREFIX}-${feature}-<sequence> and mirror Blocked by/Unblocks links across artefacts.
- Reference ${spec_rel} explicitly in the Dependencies, Evidence, and Verification sections.
- Update docs/design/README.md index, maintaining status checkboxes and dependency mirrors.
- Capture COSMIC sizing guidance and environment variable impacts; reflect findings in docs/envs/README.md or note TODOs.
- Record documentation verification details in CHANGELOG.md as required by /Users/vk/@iw2rmb/docs/AGENTS.md.

Deliverables:
1. Completed design doc written to ${design_doc_rel}.
2. Updated docs/design/README.md, docs/envs/README.md (if new env vars surface), and CHANGELOG.md.
3. Any supporting files or links needed to keep design and specs consistent.
EOF
)
    run_codex "$prompt"
  done
}

# generate_task_specs decomposes each design into task specs and refreshes the queue.
generate_task_specs() {
  log "Deriving task specs for ${#FEATURES[@]} features"
  for feature in "${FEATURES[@]}"; do
    design_doc_rel="docs/design/${feature}/README.md"
    tasks_dir_rel="docs/tasks/${feature}"
    mkdir -p "$REPO_ROOT/$tasks_dir_rel"
    log "Producing task specs in ${tasks_dir_rel} from ${design_doc_rel}"
    prompt=$(cat <<EOF
You already produced ${design_doc_rel}; now derive actionable tasks for ${INITIATIVE_PREFIX}.

Inputs:
- Design source: ${design_doc_rel}
- Tasks directory: ${tasks_dir_rel}
- Queue index: ${TASK_QUEUE_REL}
- Templates: /Users/vk/@iw2rmb/docs/templates/tasks/README.md and /Users/vk/@iw2rmb/docs/templates/tasks/INDEX.md
- COSMIC checklist: /Users/vk/@iw2rmb/docs/COSMIC.md

Requirements:
1. Create task specs under ${tasks_dir_rel} using filenames ${INITIATIVE_PREFIX}-${feature}-<sequence>-<stage>.md.
2. Ensure each task lists Blocked by/Unblocks links that mirror the design doc and other tasks.
3. Size each task with COSMIC CFPs and split anything above 4 CFP into smaller units.
4. Refresh docs/tasks/README.md using the index template so unblocked work appears first; mark newly generated tasks as unclaimed (- [ ]).
5. Update design doc dependency tables to reference the new tasks.
6. Append verification notes to CHANGELOG.md covering the planning work.

Output: Task specs ready for implementation and a dependency-sorted docs/tasks/README.md queue.
EOF
)
    run_codex "$prompt"
  done
}

# next_task_in_queue returns the first unclaimed task path from the queue.
next_task_in_queue() {
  [[ -f "$TASK_QUEUE" ]] || return 1
  local line
  while IFS= read -r line; do
    if [[ "$line" == "- [ ] "* ]]; then
      local candidate=${line#- [ ] \`}
      candidate=${candidate%\`}
      print -r -- "$candidate"
      return 0
    fi
  done < "$TASK_QUEUE"
  return 1
}

# implement_tasks runs Codex on each queued task until the backlog is empty.
implement_tasks() {
  log "Beginning task implementation loop"
  while next_task=$(next_task_in_queue); do
    log "Implementing task ${next_task}"
    prompt=$(cat <<EOF
Implement the next planned task while obeying /Users/vk/@iw2rmb/docs/AGENTS.md workflows.

Active task: ${next_task}

Execution checklist:
1. Immediately re-open ${TASK_QUEUE_REL} and mark ${next_task} as reserved (- [x]) before editing.
2. Follow the RED ➜ GREEN ➜ REFACTOR cadence spelled out in the task spec; write failing tests first.
3. Implement code, docs, and configuration updates demanded by ${next_task}, keeping design/task dependencies mirrored.
4. Run required local validation (make test, go test ./..., make build, make lint-md when docs change) and record evidence in CHANGELOG.md.
5. Update docs/design/README.md, the owning design doc, and ${TASK_QUEUE_REL} to reflect completion; remove the queue entry when done.
6. Commit with message referencing the task identifier (e.g. "feat: ${INITIATIVE_PREFIX} ${next_task}") and ensure the workspace is clean.

Do not start another task until this one is complete and committed.
EOF
)
    run_codex "$prompt"
  done
  log "Task queue empty; implementation loop finished"
}

log "Ploy v2 automation script starting"
generate_design_docs
generate_task_specs
implement_tasks
log "Ploy v2 automation script completed"
