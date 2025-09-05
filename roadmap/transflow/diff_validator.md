## Diff Validator (Orchestrator)

Before applying a diff produced by an LLM branch, validate it.

Rules
- Format: unified diff (starts with `---`/`+++` lines; contains `@@` hunks).
- Scope: changed paths must be within allowlist (e.g., `src/**`, `pom.xml`); block binary files.
- Size: cap number of files and total lines changed.
- Dry-run: attempt a `git apply --check` in a temp worktree; only proceed if it passes.

Pseudocode
```
func ValidateDiff(repoPath string, diffPath string, allow []glob) error {
  data := read(diffPath)
  if !looksLikeUnified(data) { return err }
  files := parseChangedFiles(data)
  for f in files { if !allowed(f, allow) { return err } }
  if tooLarge(data) { return err }
  if err := gitApplyCheck(repoPath, diffPath); err != nil { return err }
  return nil
}
```

On failure, mark branch `failed` and include validator error; do not attempt apply.

