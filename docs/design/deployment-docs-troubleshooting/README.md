# Deployment Docs Troubleshooting

## Why
- Operators need clear smoke tests, verification commands, and rollback steps for deployment automation.
- Documentation must stay current with dependency versions and CLI flows.

## What to do
- Document smoke tests covering bootstrap, onboarding, and rotation scenarios.
- Capture troubleshooting guidance, including common failure signatures and remediation actions.
- Publish rollback procedures and cross-reference automation docs: [`../deployment-bootstrap-host/README.md`](../deployment-bootstrap-host/README.md), [`../deployment-ca-rotation/README.md`](../deployment-ca-rotation/README.md), [`.archive/deployment-worker-onboarding/README.md`](../../../.archive/deployment-worker-onboarding/README.md).

## Where to change
- [`docs/v2/devops.md`](../../v2/devops.md) and related operator guides for detailed procedures.
- [`docs/v2/implement.sh`](../../v2/implement.sh) comments to align troubleshooting tips.
- [`docs/v2/logs.md`](../../v2/logs.md) for log collection references.

## COSMIC evaluation
| Functional process                                  | E | X | R | W | CFP |
|-----------------------------------------------------|---|---|---|---|-----|
| Document smoke tests, rollbacks, and troubleshooting | 1 | 1 | 1 | 0 | 3 |
| **TOTAL**                                           | 1 | 1 | 1 | 0 | 3 |

- Assumption: documentation updates require no new tooling; relies on existing markdown pipeline.
- Open question: confirm need for embedded diagrams or rely on textual steps only.

## How to test
- Documentation review using `markdownlint` (if enabled) on updated files.
- Manual validation: follow documented smoke tests in staging environment.
- Ensure docs reference current dependency versions before release.
