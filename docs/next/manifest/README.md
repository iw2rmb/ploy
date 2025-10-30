# Mods Step Manifest (v1)

This document defines the execution manifest for a single Mods stage. The control plane (or the CLI)
attaches a JSON manifest to the stage job metadata under the key `step_manifest`. Nodes consume this
manifest to materialize inputs, run the container, and capture outcomes.

## Goals

- Preserve the high‑level Mods abstraction:
  - Outcomes per stage: `diff`, `plan`, and `data`.
  - Inputs per stage: repository archive (by CID or repo URL/refs), ordered diffs to apply
    before execution, and auxiliary `data` (e.g., a previous plan).
- Keep the runtime portable: image, command, envs, and resources are explicit and overridable.
- Allow named step templates via `step_ref` (e.g., `orw-apply`) that pre‑define image/command.

## Format (overview)

- `schema_version` (string): must be `v1`.
- `id` (string): stage identifier (kebab‑case, 3–64 chars).
- `name` (string): human‑readable label.
- `step_ref` (string, optional): reference to a named step template (e.g., `orw-apply`).
- `inputs` (object):
  - `repo` (object, optional): materialize from VCS.
    - `url` (string): repo URL (https or ssh).
    - `base_ref` (string, optional): baseline branch/ref.
    - `target_ref` (string, optional): working branch/ref.
    - `commit` (string, optional): exact commit to checkout (overrides refs).
    - `workspace_hint` (string, optional): subdirectory to focus on.
- `snapshot` (object, optional): removed. Use `repo` and `diffs` instead. Existing manifests that
  set `snapshot.cid` will continue to hydrate from the referenced archive when routed through
  compatibility shims; new manifests should omit this field.
    - `digest` (string, optional): sha256 digest.
  - `diffs` (array, optional): ordered diffs to apply before the step (each as a commit).
    - items: `{ cid: string, digest?: string, message?: string }`.
  - `data` (array, optional): auxiliary inputs (plan, curated lists, constraints).
    - items: one of
      - `{ name: string, cid: string, media_type?: string }` (content by CID), or
      - `{ name: string, inline: { json?: any, text?: string }, media_type?: string }`.
- `runtime` (object):
  - `image` (string): container image (e.g., `ghcr.io/org/image:tag`).
  - `command` (array[string]): executable and args (preferred), or use `entrypoint` + `args`.
  - `args` (array[string], optional): additional arguments.
  - `env` (object, optional): map of `KEY: "VALUE"`.
  - `resources` (object, optional): `{ cpu: string, memory: string, disk: string }`.
- `outputs` (object):
  - `diff` (object, optional): `{ target?: string }` (defaults to the main workspace).
  - `plan` (object, optional): `{ path: string, media_type?: string }` (default `application/json`).
  - `data` (array, optional): items `{ name: string, path: string, media_type?: string }`.
- `retention` (object, optional): `{ retain_container?: boolean, ttl?: string }`.

See `../schema.json` for the full machine‑readable schema.

## Execution semantics (node)

1. Materialize the repository input:
  - From `inputs.repo` revision and optional diffs
   - From `inputs.repo` (URL + refs/commit);
   - Apply each `inputs.diffs[]` as a separate commit (preserving order).
2. Mount the realized workspace at the step’s working directory (e.g., `/workspace`).
3. Run the container (`runtime.image` + `command/args` + `env`) with the declared `resources`.
4. Capture outcomes:
   - `outputs.diff`: record a tar+zstd diff artifact (CID + digest) against the realized workspace.
   - `outputs.plan`: publish the plan file path (also as `data` if appropriate).
   - `outputs.data[]`: publish additional artifacts (logs, reports) by media type.

## Named step templates (`step_ref`)

The `step_ref` field lets manifests refer to a named step that defines `image` and `command` (and
optionally default `env`), for example `orw-apply`, `llm-plan`, `llm-exec`. The runtime section may
still override any of these values. A step registry lives server-side; the control plane resolves the
reference during job submission.

Note: The control plane may synthesize per‑stage manifests when the submission includes only the
stage graph and repository metadata. Today, the `mods-plan` stage is synthesized if no
`step_manifest` is supplied, using the repository URL and refs/commit provided at submission time.

## Examples

- `examples/llm-plan.json`: plan only (no diff); produces `/out/plan.json`.
- `examples/llm-exec.json`: consumes a plan, applies changes; outputs `diff` and data.
- `examples/orw-apply.json`: OpenRewrite apply via `step_ref`, outputs `diff` and logs.
