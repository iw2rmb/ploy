# Unified TLS & Roleless Ploy Nodes

## Why

- Worker onboarding currently fails with `deploy: cluster PKI not bootstrapped` because only the CLI can request CA generation, and that path was never wired into the new SSH-only bootstrap flow. Operators are stuck rerunning commands that can never succeed.
- The existing `PLOYD_MODE` split (“beacon” vs “worker”) duplicates binaries/configs even though every VPS host now runs the same stack (Docker, etcd, IPFS, ployd). The mode gate is purely historical and keeps causing mismatches (e.g., control-plane-only API surfaces, manual descriptor plumbing).
- Mutual TLS is still required so `ployd` instances know which peer is calling `/v1/*`, but the CA lifecycle should live entirely inside `ployd`, not in the CLI. Auto-bootstrap plus node-issued certs removes operator confusion (“why does `ploy cluster add` need PKI?”) while keeping per-node identities and revocation.
- Connection-level authorization is the right abstraction: callers (CLI, workers, health probes) should present credentials that encode their privileges. Node role should not be encoded via systemd mode switches that drift out of sync with reality.

## What to do

1. **Auto-bootstrap the cluster CA inside `ployd`.**
   - On startup, `ployd` checks etcd (e.g., `/ploy/clusters/<id>/security/current`). If absent, it uses `deploy.CARotationManager` to generate the CA, writes it via an etcd compare-and-swap, and emits a metric/log that the CA was created.
   - Subsequent instances simply read the CA; no CLI interaction is necessary.
   - Guard with leader-style locking (etcd lease or `Put` with `IfMissing`) so only the first node writes.

2. **Unify the daemon configuration.**
   - Remove `PLOYD_MODE`, `ProvisionModeBeacon/Worker`, and related script env exports. The bootstrap script always writes a single `/etc/ploy/ployd.yaml`.
   - `ployd` exposes the full API everywhere and relies on request-level auth to gate sensitive endpoints.

3. **Shift from node roles to connection roles.**
   - Certificates (or future SSH-derived tokens) carry claims such as `role=control-plane`, `role=worker`, `role=cli-admin`.
   - Introduce middleware in `internal/api/httpserver` that inspects the presented identity and enforces RBAC per route (e.g., `/v1/nodes` requires `control-plane` or `cli-admin`, `/v1/status` allows read-only clients).
   - Worker-issued certs still come from the same CA, but their role claim is “worker” instead of deriving behavior from systemd units.

4. **Simplify `ploy cluster add`.**
   - Bootstrap path: copy binaries, converge deps, run the unified script. No hidden CA step.
   - Worker path: same script, then call `/v1/nodes` over the SSH tunnel; the server now has the CA ready and responds with worker certs/labels.
   - Drop worker-only flag validation (labels/probes allowed on all runs) and remove stale messaging about “beacon vs worker.”

5. **Documentation & tooling.**
   - Update `docs/next/devops.md` and `docs/next/vps-lab.md` to describe the new flow (“bootstrap host → ployd auto-generates CA → add additional nodes”).
   - Document connection-role expectations for external orchestrators (how to obtain an admin cert/token).
   - Extend CLI/docs to surface CA state (e.g., `ploy cluster cert status`) once auto-bootstrap lands.

## Where to change

- [`internal/bootstrap/assets/bootstrap.sh`](../../../internal/bootstrap/assets/bootstrap.sh), [`internal/deploy/provision.go`](../../../internal/deploy/provision.go), [`cmd/ploy/cluster_add.go`](../../../cmd/ploy/cluster_add.go), [`internal/deploy/bootstrap.go`](../../../internal/deploy/bootstrap.go) — remove mode/env plumbing and ensure the bootstrap script is role-agnostic.
- [`internal/controlplane/security`](../../../internal/controlplane/security), [`internal/deploy/ca_rotation.go`](../../../internal/deploy/ca_rotation.go), [`internal/api/controlplane`](../../../internal/api/controlplane) — add automatic CA creation and refactor cert issuance/validation to rely on connection roles.
- [`pkg/sshtransport`](../../../pkg/sshtransport), [`cmd/ploy/cluster_command_test.go`](../../../cmd/ploy/cluster_command_test.go), [`docs/next/devops.md`](../../next/devops.md), [`docs/next/vps-lab.md`](../../next/vps-lab.md) — align tunnels/tests/docs with the new assumptions (root default user, no manual PKI step).
- [`internal/api/httpserver`](../../../internal/api/httpserver), new `internal/controlplane/auth` (or equivalent) — introduce middleware for RBAC based on presented client identity.

## COSMIC evaluation

| Functional process                               | E | X | R | W | CFP |
|--------------------------------------------------|---|---|---|---|-----|
| Auto-bootstrap CA on ployd startup               | 1 | 1 | 1 | 1 | 4   |
| Register node connection roles & enforce RBAC    | 1 | 1 | 2 | 1 | 5   |
| Simplified `ploy cluster add` / descriptor flow  | 2 | 1 | 1 | 1 | 5   |
| TOTAL                                            |   |   |   |   | 14  |

Assumptions: a single etcd cluster per deployment; RBAC middleware reuses existing TLS metadata; CLI changes do not add new persistent stores beyond descriptors. Open questions: whether external identity providers (e.g., Nomad) need their own role mapping; how to expose CA bootstrap telemetry to operators.

## How to test

- Unit tests:
  - `internal/controlplane/security`: simulate empty etcd and ensure the CA auto-bootstrap path writes once, subsequent startups reuse it.
  - `internal/api/httpserver`: verify RBAC middleware blocks disallowed roles and allows valid ones.
  - `cmd/ploy/cluster_command_test.go`: ensure cluster add flows work without `PLOYD_MODE`, and descriptor base URLs honor new defaults.
- Integration / VPS lab:
  - Bootstrap a fresh node; confirm `/ploy/clusters/<id>/security/current` appears without manual commands.
  - Add additional nodes; verify worker registration succeeds and issued certs land on disk.
  - Hit `/v1/status`, `/v1/nodes`, `/v1/config` with different credentials (CLI admin vs worker cert) to ensure RBAC behaves.
