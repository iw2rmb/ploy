# Runs

`ploy run` is the single-repo run command. It submits one mig spec against one
repository source and then exposes explicit inspection, artifact download, and
patch-application commands.

## Submit

```bash
ploy run <spec-path> [<repo-path>|<namespace/repo[:ref]>] [--apply] [--pull[=path]]
```

- `<spec-path>` is required. A directory resolves to `<spec-path>/mig.yaml`.
- If the repo argument is omitted, `.` is used.
- A local repo path submits the selected worktree's `HEAD`. Local staged and
  unstaged changes are ignored during submit.
- A remote selector uses `namespace/repo`, `namespace/repo:<branch>`, or
  `namespace/repo:<sha>`. The default ref is `master`.
- Remote selector expansion is server-owned through `POST /v1/repos/resolve`.
  The CLI does not infer the repository host from local config.
- `--apply` is allowed only for local repo submissions.
- `--pull` waits for a successful terminal state and downloads final artifacts.
  Bare `--pull` creates a temporary directory; `--pull=<path>` uses that path.

Submit prints the created `run_id` and `mig_id` when it returns immediately. When
`--pull` or `--apply` is used, the command follows the run until a terminal state
because both operations depend on successful completion.

## Inspect

```bash
ploy run status <run-id>
ploy run status <run-id> --json
ploy run status <run-id> --follow
ploy run ls [<path>|<namespace/repo[:ref]>] [--limit N] [--offset N]
ploy run cancel <run-id>
```

- `status` renders the current run report.
- `status --json` prints the same report as JSON.
- `status --follow` polls report/status endpoints until the run reaches a
  terminal state. It does not depend on a run-level log stream.
- `ls` lists recent runs globally or for a resolved repo selector.

Container logs are job-scoped:

```bash
ploy job log <job-id>
ploy job log --format structured <job-id>
ploy job log --follow <job-id>
```

The default job log format is raw. Use `--format structured` when timestamps and
stream labels are needed.

## Artifacts

```bash
ploy run pull <run-id> [artifacts-path]
```

`run pull` downloads final artifacts and writes `manifest.json` into the selected
directory. It does not inspect a git worktree, create branches, or apply diffs.

## Apply

```bash
ploy run apply <run-id> [path] [--force]
```

`run apply` applies the accumulated patch for the run repo into a local git
worktree.

Rules:

- The target path must be inside a git worktree.
- The worktree must have no staged or unstaged diff against `HEAD`.
- Local `HEAD` must match the run repo `source_commit_sha`.
- `--force` bypasses only the `HEAD` versus `source_commit_sha` guard.
- `--force` never bypasses the clean-worktree guard.
- The command does not create or switch branches.

## Compatibility

The current command surface has no compatibility aliases for older submit,
manual-start, run-log, or patch-download shapes. Contract acceptance is defined
by the current Cobra command tree and the current API schema only.
