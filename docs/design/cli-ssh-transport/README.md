# CLI SSH Transport

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 2 of 3 (`SSH Transport Pivot`)

## Summary
Refactor the Ploy CLI to route all control-plane traffic through persistent SSH tunnels. Reuse existing HTTP/gRPC clients by forwarding `localhost` ports per node. Secure access via SSH keys seeded during node enrollment and maintain a local cache of node/job assignments.

## Goals
- Implement an SSH tunnel manager (ControlMaster-equivalent) in the CLI.
- Replace direct HTTPS clients with tunnel-aware HTTP/gRPC clients.
- Cache node/job bindings to target the correct tunnel for follow-up requests.
- Handle reconnects and node failover transparently.

## Non-Goals
- Server-side SSH setup (covered in `controlplane-ssh-surface`).
- Artifact upload/download channels (see `cli-ssh-artifacts`).

## Current State
- CLI talks directly to beacon-hosted HTTPS endpoints using TLS certificates.
- No persistent connections or local node/job cache.

## Proposed Changes
1. **Tunnel Manager**
   - `pkg/sshtransport` managing persistent tunnels using `golang.org/x/crypto/ssh` or external `ssh` binary.
   - Each known node IP gets a tunnel with `LocalForward <port>:localhost:<api-port>`.
   - Control sockets stored under `~/.ploy/tunnels`.
2. **Client Integration**
   - Wrap REST/gRPC clients with a new constructor that requests a tunnel and receives the forwarded address.
   - Inject job cache lookups before each request to pick the right node.
3. **Cache Layer**
   - Persist node list and job assignments locally (JSON/YAML).
   - Update cache on responses (job submission, status sync endpoint).
4. **Recovery**
   - Automatic reconnect with exponential backoff when tunnel drops.
   - Fallback to next available node for generic requests.

## Work Plan
1. Implement tunnel manager with unit tests around connect/failover.
2. Modify CLI clients/commands to use the tunnel manager.
3. Add job cache persistence and CLI flags for inspection/reset.
4. Integrate status sync endpoint (once server provides) to refresh cache.

## Testing Strategy
- Unit: mock SSH dialing, ensure manager recovers from disconnects.
- Integration: run CLI against loopback server via `ssh -L` harness in CI.
- CLI E2E: submit job, poll status, queue operations entirely through tunnels.

## Documentation
- Document new CLI config files, control sockets, failure modes.
- Update operator guides to explain tunnel behavior and diagnostics.

## Dependencies
- Depends on `controlplane-ssh-surface` to expose SSH access and loopback services.
