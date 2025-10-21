#!/usr/bin/env zsh

set -euo pipefail

script_dir="${0:A:h}"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
cd "$repo_root"

if codex exec --help | grep -q -- '--search'; then
  codex_has_search=1
else
  codex_has_search=0
  echo "Warning: codex exec does not support --search; continuing without scoped search." >&2
fi

run_codex() {
  local search_scope="$1"
  local prompt_payload="$2"

  if (( codex_has_search )); then
    print -r -- "$prompt_payload" | codex exec --dangerously-bypass-approvals-and-sandbox --search "$search_scope"
  else
    print -r -- "$prompt_payload" | codex exec --dangerously-bypass-approvals-and-sandbox
  fi
}

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

design_prompt=$(cat <<EOF
You are the lead architect for the docs/v2 program. Generate the full scope of design documentation required for the docs/v2 implementation. Review every existing file under docs/v2, add any missing design artifacts, and ensure each document is complete, internally consistent, and cross-referenced. Where new documents are needed, create them under docs/v2 following existing formatting conventions.

Current docs inventory:
${design_doc_list}

Deliver updated and newly created design docs directly in the repository.
EOF
)

run_codex "docs/v2 docs/tasks" "$design_prompt"

design_docs=(${(@f)$(find docs/v2 -maxdepth 1 -type f -name '*.md' | sort)})

for doc in "${design_docs[@]}"; do
  doc_name="${doc:t}"
  doc_slug="${doc_name%.md}"
  task_file="docs/tasks/${doc_slug}.md"

  task_prompt=$(cat <<EOF
Derive the implementation task plan for ${doc}. Produce a sequenced checklist of actionable tasks with acceptance criteria and owner notes, then write the plan to ${task_file}. Each checklist item should map to a concrete deliverable that moves the docs/v2 implementation forward.
EOF
  )

  run_codex "${doc} docs/tasks" "$task_prompt"
done

queue_prompt=$(cat <<'EOF'
Update docs/tasks/README.md to reflect the current task queue generated for the docs/v2 rollout. Preserve the legend, add each docs/tasks/*.md file in dependency order as unchecked items, and remove any stale entries.
EOF
)

run_codex "docs/tasks" "$queue_prompt"

typeset -a task_files
task_files=(${(@f)$(find docs/tasks -maxdepth 1 -type f -name '*.md' ! -name 'README.md' | sort)})

if (( ${#task_files[@]} == 0 )); then
  echo "No task files found under docs/tasks; skipping implementation phase." >&2
  exit 0
fi

for task in "${task_files[@]}"; do
  impl_prompt=$(cat <<EOF
Implement the work described in ${task}. Complete the checklist items, update code, tests, and docs as required, run the necessary validations, and then create a focused git commit referencing ${task}. Provide a brief implementation report in the command output.
EOF
  )

  run_codex "${task} docs/v2 internal pkg cmd" "$impl_prompt"
done
