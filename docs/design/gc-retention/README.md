# GC Retention

## Why
- Ploy must enforce retention policies for job metadata, containers, logs, and diff bundles so nodes avoid storage exhaustion.
- Inspection workflows depend on predictable retention windows and audit trails for every deletion.
- Operators need manual overrides (`ploy gc`) for accelerated cleanup or debugging without risking active jobs.

## What to do
- Implement a GC controller that scans etcd `gc/jobs/<job-id>` markers, unpins expired IPFS artifacts, prunes node containers, and removes job state safely.
- Add CLI support for manual sweeps, dry runs, per-job overrides, and reporting on deletions versus failures.
- Emit audit logs and Prometheus metrics covering GC actions, failures, and skipped items for inspection-ready jobs.

## Where to change
- [`internal/controlplane/gc`](../../../internal/controlplane/gc) (new) for the controller loop integrating etcd markers, IPFS unpin logic, and node prune RPCs.
- [`cmd/ploy/gc`](../../../cmd/ploy/gc) for manual command entry points and output formatting.
- [`docs/v2/gc.md`](../../v2/gc.md), [`docs/v2/logs.md`](../../v2/logs.md), and operator runbooks for retention windows, overrides, and troubleshooting guidance.

## COSMIC evaluation
| Functional process           | E | X | R | W | CFP |
|------------------------------|---|---|---|---|-----|
| Sweep expired GC markers     | 1 | 1 | 2 | 2 | 6   |
| Execute manual GC sweep      | 1 | 1 | 1 | 1 | 4   |
| Publish GC audit metrics     | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                    | 3 | 3 | 4 | 3 | 13  |

- Assumptions: artifact unpinning and node prune RPCs count as separate writes; audit metrics expose results via Prometheus without extra storage.
- Open questions: verify whether retention overrides require additional writes to track exceptions per job.

## How to test
- `go test ./internal/controlplane/gc/...` covering expiration selection, retry logic, and failure handling.
- Integration runs: create jobs with short retention, verify GC unpins artifacts, prunes containers, and preserves inspection-ready entries until overrides expire.
- CLI smoke: `make build && dist/ploy gc --dry-run` to validate reporting and dry-run safeguards.
