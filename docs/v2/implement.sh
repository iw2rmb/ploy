#!/bin/zsh

set -euo pipefail

script_dir="${0:A:h}"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
cd "$repo_root"

typeset -a design_docs
design_docs=(${(@f)$(find docs/v2 -maxdepth 1 -type f -name '*.md' | sort)})

if (( ${#design_docs[@]} == 0 )); then
  echo "No design docs found in docs/v2; aborting." >&2
  exit 1
fi

design_doc_list=""
for doc in "${design_docs[@]}"; do
  design_doc_list+="- ${doc}"$'\n'
done

codex exec \
  --dangerously-bypass-approvals-and-sandbox \
  --search "docs/v2 docs/tasks" <<EOF
You are the lead architect for the docs/v2 program. Generate the full scope of design documentation required for the docs/v2 implementation. Review every existing file under docs/v2, add any missing design artifacts, and ensure each document is complete, internally consistent, and cross-referenced. Where new documents are needed, create them under docs/v2 following existing formatting conventions.

Current docs inventory:
${design_doc_list}

Deliver updated and newly created design docs directly in the repository.
EOF

for doc in "${design_docs[@]}"; do
  doc_name="${doc:t}"
  doc_slug="${doc_name%.md}"
  task_file="docs/tasks/${doc_slug}.md"

  codex exec \
    --dangerously-bypass-approvals-and-sandbox \
    --search "${doc} docs/tasks" <<EOF
Derive the implementation task plan for ${doc}. Produce a sequenced checklist of actionable tasks with acceptance criteria and owner notes, then write the plan to ${task_file}. Each checklist item should map to a concrete deliverable that moves the docs/v2 implementation forward.
EOF
done

codex exec \
  --dangerously-bypass-approvals-and-sandbox \
  --search "docs/tasks" <<'EOF'
Update docs/tasks/README.md to reflect the current task queue generated for the docs/v2 rollout. Preserve the legend, add each docs/tasks/*.md file in dependency order as unchecked items, and remove any stale entries.
EOF

typeset -a task_files
task_files=(${(@f)$(find docs/tasks -maxdepth 1 -type f -name '*.md' ! -name 'README.md' | sort)})

if (( ${#task_files[@]} == 0 )); then
  echo "No task files found under docs/tasks; skipping implementation phase." >&2
  exit 0
fi

for task in "${task_files[@]}"; do
  codex exec \
    --dangerously-bypass-approvals-and-sandbox \
    --search "${task} docs/v2 internal pkg cmd" <<EOF
Implement the work described in ${task}. Complete the checklist items, update code, tests, and docs as required, run the necessary validations, and then create a focused git commit referencing ${task}. Provide a brief implementation report in the command output.
EOF
done
