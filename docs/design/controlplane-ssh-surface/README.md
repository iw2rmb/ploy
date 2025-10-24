# Control Plane SSH Surface

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 1 of 3 (`SSH Transport Pivot`)

## Summary
Expose each workflow node via SSH, bind existing HTTP/gRPC services to loopback, and retire the HTTPS beacon. Nodes accept only key-based authentication, and admin/user roles are separated through `authorized_keys` restrictions.

## Goals
- Install and harden `sshd` (or embedded `gliderlabs/ssh`) on every control-plane node.
- Restrict control-plane APIs to localhost interfaces.
- Seed role-scoped SSH keys during node enrollment.
- Emit telemetry for tunnel connects/disconnects to replace beacon metrics.

## Non-Goals
- CLI transport refactor (covered in `cli-ssh-transport`).
- Artifact transfer mechanics (covered in `cli-ssh-artifacts`).

## Current State
- Nodes expose HTTPS endpoints publicly behind the beacon.
- Certificate issuance/rotation handled by the beacon CA.
- Node telemetry depends on beacon routing.

## Proposed Changes
1. **SSHD Enablement**
   - Update bootstrap scripts to ensure `sshd` is enabled, listens on node’s routable IP, enforces `PasswordAuthentication no`.
   - Load host keys from configuration management; rotate on schedule.
2. **Loopback HTTP Binding**
   - Reconfigure control-plane services to listen on `127.0.0.1:<port>`.
   - Update health checks to probe via localhost.
3. **Key Management**
   - Maintain `admin_authorized_keys` and `user_authorized_keys`.
   - Admin keys may call node enrollment endpoints; user keys restricted to job control.
4. **Telemetry**
   - Ship logs/metrics on SSH session connect/disconnect.
   - Document monitoring hooks (e.g., journald, Prometheus exporters).

## Work Plan
1. Extend bootstrap scripts and AMI/container images with SSH hardening.
2. Update service configs for loopback binding; adjust systemd units.
3. Implement enrollment flow that uploads admin/user keys.
4. Configure logging/metrics pipelines for SSH events.

## Testing Strategy
- Unit: config templating tests for `sshd_config`, service binding.
- Integration: provisioned node smoke test ensuring HTTP unreachable externally, reachable via `ssh -L`.
- Security: verify sshd rejects password auth, tracks session metadata.

## Documentation
- Update runbooks (deployment, incident response) with SSH access steps.
- Remove beacon references from node provisioning docs.

## Dependencies
- None; prepares groundwork for CLI transport and artifacts.
