# SBOMs

## Goal

Store gate-produced outputs from `/out/*` without repository writes, persist SBOM package rows, and expose compatibility lookup inputs for `deps` healing.

## Scope

- Build Gate artifact handling.
- SBOM artifact discovery and parsing from gate `/out`.
- SBOM package persistence model.
- SBOM-backed compatibility query contract.

## Required Runtime Behavior

- Gate execution must not require modifying repository files.
- Gate-generated files must be written to `/out/*`.
- Nodeagent must persist gate `/out/*` artifacts in general, not only tool-specific report paths.
- SBOM rows are persisted only for successful gate jobs (`pre_gate`, `post_gate`, `re_gate`).

## `/out` Artifact Storage (General)

- Treat gate `/out` as a first-class artifact source, same model as other job types.
- Upload all files under `/out` with deterministic archive paths rooted at `out/`.
- Keep path fidelity so downstream processors can resolve exact artifact file locations.

## SBOM Handling (Particular)

- Detect SBOM files inside uploaded gate `/out/*` artifacts.
- Parse supported SBOM formats and flatten package entries.
- Bind each parsed package to the producing `job_id` and `repo_id`.

## Data Model

Doc table:
- `sboms(job_id, repo_id, lib, ver)`

Column intent:
- `job_id`: producer job identity.
- `repo_id`: repository identity.
- `lib`: normalized package/library name.
- `ver`: normalized package/library version.

Stack/time are intentionally derived by joins, not duplicated in `sboms`:

- Time: `sboms.job_id -> jobs.id -> jobs.created_at`
- Stack: `sboms.job_id -> gates.job_id -> gate_profiles.stack_id -> stacks`

## Compatibility Query Contract

Endpoint:

- `GET /v1/sboms/compat?lang=<lang>&release=<release>&tool=<tool>&libs=<name>:<ver>,<name>`

Behavior:

- Resolve stack filter via joins:
  - `sboms.job_id -> gates -> gate_profiles -> stacks`
- Filter only successful gate jobs via `sboms.job_id -> jobs.status='Success'`.
- For each requested lib:
  - `name`: return minimum observed successful version.
  - `name:ver`: return minimum observed successful version that is `>= ver`.
- Return object payload: `{ "<lib>": "<ver>", ... }`.
- Return `null` when no successful SBOM evidence exists for requested stack.

Version ordering must be ecosystem-aware, not lexical string comparison.

## Integration with `deps` Bumps

- Dependency bump state is stored only in healing/gate `jobs.meta.recovery.deps_bumps`.
- `sboms` does not store `deps_bumps`.
- `deps` healing receives prior bumps from job metadata and compatibility hints from `/v1/sboms/compat`.

## Related Docs

- `design/bumps.md`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
- `design/gate-profile.md`
