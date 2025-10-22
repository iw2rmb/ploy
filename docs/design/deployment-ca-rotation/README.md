# Deployment CA Rotation

## Why
- Ploy deployments require automated CA material generation and rotation to secure node communication.
- Operators need safe rotation hooks that avoid downtime across beacon and worker nodes.

## What to do
- Generate CA, beacon, and worker certificates during bootstrap with secure storage.
- Implement rotation commands (`ploy beacon rotate-ca`, etc.) that distribute new materials and revoke old ones.
- Coordinate with credential flows defined in [`../deployment-worker-onboarding/README.md`](../deployment-worker-onboarding/README.md).

## Where to change
- [`internal/deploy`](../../../internal/deploy) for certificate generation and distribution logic.
- [`cmd/ploy/deploy`](../../../cmd/ploy/deploy) to wire rotation commands.
- [`internal/controlplane/security`](../../../internal/controlplane/security) (or similar) for trust store updates.
- Documentation updates in [`docs/v2/devops.md`](../../v2/devops.md) covering rotation procedures.

## COSMIC evaluation
| Functional process                           | E | X | R | W | CFP |
|----------------------------------------------|---|---|---|---|-----|
| Generate and rotate CA plus cluster descriptors | 1 | 1 | 1 | 1 | 4 |
| **TOTAL**                                    | 1 | 1 | 1 | 1 | 4 |

- Assumption: rotation stores state in etcd alongside existing cluster metadata.
- Open question: confirm downtime requirements and whether rolling reload is acceptable.

## How to test
- `go test ./internal/deploy -run TestCARotation` covering cert issuance and revocation markers.
- Integration: perform rotation in staging cluster, confirm nodes reconnect with new certs.
- Smoke: `make build && dist/ploy beacon rotate-ca --dry-run` verifying preview output.
