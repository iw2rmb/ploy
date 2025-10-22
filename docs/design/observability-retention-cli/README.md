# Observability Retention CLI

## Why
- Operators need visibility into bundle retention metadata, CIDs, and TTLs from the CLI.
- Job summaries must surface retention fields so inspection-ready jobs are easy to audit.

## What to do
- Record retention metadata in job summaries and expose it through control plane APIs.
- Update CLI commands to show bundle CIDs, retention windows, and inspection hints (see [`../cli-streaming/README.md`](../cli-streaming/README.md)).
- Document operator workflows referencing log bundles per [`../observability-log-bundles/README.md`](../observability-log-bundles/README.md).

## Where to change
- [`internal/controlplane/httpapi`](../../../internal/controlplane/httpapi) to return retention metadata in job responses.
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler) for TTL updates after bundle persistence.
- [`cmd/ploy/mods`](../../../cmd/ploy/mods) and related CLI packages to display retention fields.
- [`docs/v2/logs.md`](../../v2/logs.md) and [`docs/v2/devops.md`](../../v2/devops.md) for operator guidance.

## COSMIC evaluation
| Functional process                               | E | X | R | W | CFP |
|--------------------------------------------------|---|---|---|---|-----|
| Record retention metadata and surface CLI fields | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                                        | 1 | 1 | 1 | 0 | 3   |

- Assumption: retention metadata stores within existing job summary objects.
- Open question: confirm TTL adjustments propagate to GC controller (see [`../gc-marker-controller/README.md`](../gc-marker-controller/README.md)).

## How to test
- `go test ./internal/controlplane/httpapi -run TestJobRetention` to validate metadata exposure.
- CLI snapshot tests showing retention fields in output.
- Manual smoke: run job, verify CLI prints bundle CID and TTL, confirm GC respects TTL.
