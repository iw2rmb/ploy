Migs Contract

This directory contains example Migs images (container build contexts). Ploy does not know what a mig does; it only runs your container with a mounted Git workspace and collects outputs you choose to emit. Containers must follow a small, stable runtime contract described here.

Contract

- Repo workspace
  - Mount path inside the container: `/workspace` (read–write).
  - Contents: a shallow Git checkout of the requested repository/ref. `HEAD` points to the input commit and the working tree is clean when the container starts.
  - Diff semantics: to produce a change set, modify files under `/workspace` without committing them. Ploy detects the output diff as uncommitted changes (equivalent to `git diff HEAD`) after the container exits.

- Input artifacts (optional)
  - Mount path: `/in` (read-only) when present.
  - Contents: any auxiliary inputs (plans, configs) that orchestration chooses to provide. Not all runs have inputs; your mig must tolerate `/in` being empty or absent.

- Output artifacts (optional)
  - Mount path: `/out` (read–write, empty directory for each run).
  - Write any machine‑readable reports or logs you want to keep into `/out`. Ploy bundles the contents of `/out` as an artifact bundle after the container exits.
  - Do not commit output files to Git; keep your repository changes uncommitted so the diff represents your transformation.

- Logs and exit codes
  - Write logs to stdout/stderr. Ploy streams them live and may persist them.
  - Exit code `0` indicates success; non‑zero signals failure. Choose exit codes consistently; Ploy treats non‑zero as a failed stage.

Fixed Paths (summary)

- `/workspace` — Git working tree (RW). Modify here to produce the diff (uncommitted changes).
- `/in` — Optional inputs (RO).
- `/out` — Collected outputs (RW). Ploy uploads its contents as a bundle.

Notes

- Ploy is transport‑only: image, command, and env are provided by the operator and passed through unchanged. Avoid relying on hidden defaults.
- If you also want Ploy to bundle files under `/workspace`, coordinate with ops to supply `artifact_paths` in the run spec. `/out` is always bundled automatically.
  - `artifact_paths` must be workspace-relative (no absolute paths like `/etc/passwd`).
  - Path traversal that escapes `/workspace` (e.g. `../../etc/passwd`) is rejected and skipped.
