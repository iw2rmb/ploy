# Job Field Contract for `next_id` Chains

Status: current migration contract for `roadmap/next.md` Phase 0.

## Scope

This document defines the canonical job field names and job type values for the
`step_index` -> `next_id` migration slice.

It applies to interface surfaces that currently expose job execution metadata:

- `internal/nodeagent/job.go`
- `internal/workflow/contracts/job_meta.go`
- `docs/api/components/schemas/controlplane.yaml`

## Canonical Field Mapping

Use the new names everywhere in the migrated contract:

| Legacy name | Canonical name | Go type alias |
| --- | --- | --- |
| `ModType` / `mod_type` | `Type` / `job_type` (or `type` where schema already uses `type`) | `JobType` |
| `ModImage` / `mod_image` | `Image` / `job_image` (or `image` where schema already uses `image`) | `JobImage` |
| `step_index` | `next_id` chain links | n/a |

The legacy phase names `pre_gate`, `mod`, `post_gate`, and `re_gate` are not
part of this new contract.

## Canonical `JobType` Values

`JobType` values are step phases:

- `pre_build`
- `step`
- `post_build`
- `heal`
- `re_build`
- `mr`

These values replace old phase labels in new interfaces for this migration.

## Chain Contract (`next_id`)

Jobs are linked by explicit successor pointers:

- `jobs.next_id` is nullable and references `jobs.id`.
- A chain head is a job that is not referenced by any other job as `next_id`.
- A chain tail has `next_id = NULL`.
- Healing insertion rewires links by replacing one edge with two edges:
  failed job -> first healing job -> previous successor.

Ordering is defined by link traversal, not by numeric sorting.

## References

- `roadmap/next.md`
- `roadmap/mig.md`
- `docs/mods-lifecycle.md`
- `docs/api/OpenAPI.yaml`
