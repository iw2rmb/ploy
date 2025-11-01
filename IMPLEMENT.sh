#!/usr/bin/env bash
set -euo pipefail

# IMPLEMENT automation runner (folder-aware)
# - Operates on a target folder containing ROADMAP.md
# - Uses GOLANG.md from monorepo root (where this script lives)
# - Calls Claude to implement next unchecked task and Codex to review
# - Includes the last "## Summary" cut from .claude/loop/claude.log (in the target folder) in the Codex prompt

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
MONO_ROOT="$SCRIPT_DIR"

TARGET_DIR="${1:-$PWD}"
TARGET_DIR="$(cd -- "$TARGET_DIR" && pwd -P)"

REF_MD="$TARGET_DIR/ROADMAP.md"
GOLANG_MD_PATH="$MONO_ROOT/GOLANG.md"

if [[ ! -f "$GOLANG_MD_PATH" ]]; then
  echo "[implement] Could not find GOLANG.md at monorepo root: $GOLANG_MD_PATH" >&2
  exit 1
fi

# Prefer the nearest git repo that contains the target folder
REPO_DIR="$(git -C "$TARGET_DIR" rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "${REPO_DIR:-}" ]]; then
  REPO_DIR="$(git -C "$MONO_ROOT" rev-parse --show-toplevel 2>/dev/null || echo "$MONO_ROOT")"
fi

LOG_DIR="$TARGET_DIR/.claude/loop"
mkdir -p "$LOG_DIR"

CLAUDE_CMD="claude"
CODEX_CMD="codex"

have_claude() { command -v "$CLAUDE_CMD" >/dev/null 2>&1; }
have_codex() { command -v "$CODEX_CMD" >/dev/null 2>&1; }
die() { echo "[implement] $*" >&2; exit 1; }

require_tools() {
  have_claude || die "Claude CLI not found. Please install 'claude'."
  have_codex || die "Codex CLI not found. Please install 'codex'."
  command -v git >/dev/null 2>&1 || die "git not found"
  [[ -f "$REF_MD" ]] || die "Missing $REF_MD in target folder: $TARGET_DIR"
}

require_clean_tree() {
  if ! git -C "$REPO_DIR" diff --quiet --ignore-submodules --exit-code || \
     ! git -C "$REPO_DIR" diff --cached --quiet --ignore-submodules --exit-code; then
    echo "[implement] Working tree is not clean in $REPO_DIR." >&2
    git -C "$REPO_DIR" -c color.status=always status --short || true
    exit 1
  fi
}

first_unchecked_task() {
  LC_ALL=C awk '
    /^[ \t]*-[ \t]*\[ \][ \t]*/ {
      line=$0; sub(/^[ \t]*-[ \t]*\[ \][ \t]*/, "", line); print NR"\t"line; exit 0
    }
  ' "$REF_MD"
}

extract_title() { perl -CS -pe 's/[ \t]+[—-][ \t]*Purpose:.*$//;' | sed -E 's/[ \t]+$//'; }

last_commit_pretty() {
  if git -C "$REPO_DIR" rev-parse --verify HEAD >/dev/null 2>&1; then
    git -C "$REPO_DIR" log -1 --pretty=format:%H" %s"
  else
    echo "no commits yet"
  fi
}

extract_last_summary() {
  local logf="$LOG_DIR/claude.log"
  # Backward compatibility: fall back to old path if needed
  if [[ ! -f "$logf" ]] && [[ -f "$TARGET_DIR/.claude/loop/claude.log" ]]; then
    logf="$TARGET_DIR/.claude/loop/claude.log"
  fi
  [[ -f "$logf" ]] || { echo "<no previous Claude log found at $logf>"; return; }
  awk '
    /^##[[:space:]]+Summary[[:space:]]*$/ { last=NR }
    { lines[NR]=$0 }
    END {
      if (!last) { print "<no ## Summary section found>"; exit }
      for (i=last; i<=NR; i++) {
        if (i>last && lines[i] ~ /^##[[:space:]]+/) break;
        print lines[i]
      }
    }
  ' "$logf"
}

build_prompt_impl() {
  local line_no="$1"; shift
  local raw_task="$*"
  local title
  title="$(printf "%s" "$raw_task" | extract_title)"

  local pf
  pf="$LOG_DIR/impl_$(date +%Y%m%d_%H%M%S).prompt.txt"

  {
    echo "You are an autonomous code agent with write access to this repository."
    echo "Working directory: $TARGET_DIR"
    echo "Task: Implement the FIRST unchecked checklist item from ROADMAP.md at line $line_no."
    echo "Exact task name: $title"
    echo
    echo "Constraints:"
    echo "- Strictly follow engineering rules in: $GOLANG_MD_PATH"
    echo "- Scope **ONLY** this task; **DO NOT** modify unrelated items/files."
    echo "- Make minimal, well-scoped changes; add/update tests as required."
    echo "- **DO NOT** run repo-wide format/lint/race checks (pre-commit/CI handle these)."
    echo "- **DO NOT** call 'goimports' or 'gofmt' directly; pre-commit applies formatting on commit."
    echo "- Limit testing to fast unit/package tests for changed code (omit '-race' repo-wide)."
    echo "- Keep output concise: delta-style summary (goal, files, tests, result)."
    echo "- When done, in ROADMAP.md change only that line's checkbox to '[x]'."
    echo
    echo "Reference files to read (do not inline contents):"
    echo "- $GOLANG_MD_PATH"
    echo "- $REF_MD"
    echo
    echo "Deliverables:"
    echo "- Apply code and test changes directly in the repo."
    echo "- Mark only the targeted item as completed in ROADMAP.md."
  } > "$pf"

  printf "%s" "$pf"
}

build_prompt_review() {
  local title="$1"
  local last_commit
  last_commit="$(last_commit_pretty)"
  local summary_cut
  summary_cut="$(extract_last_summary)"
  local pf
  pf="$LOG_DIR/review_$(date +%Y%m%d_%H%M%S).prompt.txt"

  {
    echo "You are performing a focused review/fix pass for the most recent ROADMAP.md task implementation in: $TARGET_DIR"
    echo "Last commit: $last_commit"
    echo "Target task: $title"
    echo
    echo "What changed (from last Claude run log):"
    echo "-----"
    echo "$summary_cut"
    echo "-----"
    echo
    echo "Goals:"
    echo "- Verify alignment with $GOLANG_MD_PATH."
    echo "- You own repo-wide checks (summarize results, do not paste long logs):"
    echo "  * go vet ./... && staticcheck ./..."
    echo "  * go test -race ./..."
    echo "- Do **NOT** call 'goimports' or 'gofmt' (pre-commit formats on commit)."
    echo "- Add edge/fuzz/integration tests as needed; prefer minimal corrections over rewrites."
    echo "- Keep log output compact: one-line status per tool + short delta (files changed, rationale)."
    echo "- Do not alter other unchecked items in ROADMAP.md."
    echo
    echo "Reference files to read (do not inline contents):"
    echo "- $GOLANG_MD_PATH"
    echo "- $REF_MD"
    echo
    echo "Deliverables:"
    echo "- Apply any required code/test/doc changes directly."
  } > "$pf"

  printf "%s" "$pf"
}

run_claude() {
  local prompt_file="$1"
  ( cd "$TARGET_DIR" && "$CLAUDE_CMD" --permission-mode bypassPermissions --dangerously-skip-permissions -p < "$prompt_file" ) | tee -a "$LOG_DIR/claude.log"
}

run_codex() {
  local prompt_file="$1"
  ( cd "$TARGET_DIR" && "$CODEX_CMD" exec --yolo - < "$prompt_file" ) | tee -a "$LOG_DIR/codex.log"
}

commit_if_changes() {
  local message="$1"
  if ! git -C "$REPO_DIR" diff --quiet --no-ext-diff; then
    git -C "$REPO_DIR" add -A
    git -C "$REPO_DIR" commit -m "$message"
  fi
}

require_tools
require_clean_tree

# If managed git hooks are present, point core.hooksPath to .githooks so
# formatting runs automatically on commits made by this script and developers.
if [[ -d "$MONO_ROOT/.githooks" ]]; then
  current_hooks_path="$(git -C "$REPO_DIR" config --get core.hooksPath || true)"
  if [[ "$current_hooks_path" != ".githooks" ]]; then
    echo "[implement] Configuring git hooks path to .githooks (one-time)."
    git -C "$REPO_DIR" config core.hooksPath .githooks || true
  fi
fi

while :; do
  require_clean_tree

  if ! out="$(first_unchecked_task)" || [[ -z "$out" ]]; then
    echo "[implement] No unchecked tasks in $REF_MD — exiting."
    exit 0
  fi

  line_no="${out%%$'\t'*}"
  raw_task="${out#*$'\t'}"
  title="$(printf "%s" "$raw_task" | extract_title)"

  echo "[implement] Next task (line $line_no): $title"

  impl_prompt="$(build_prompt_impl "$line_no" "$raw_task")"
  echo "[implement] Calling Claude to implement…"
  run_claude "$impl_prompt"

  commit_if_changes "$title"

  review_prompt="$(build_prompt_review "$title")"
  echo "[implement] Calling Codex to review…"
  run_codex "$review_prompt"

  commit_if_changes "Review: $title"
done
