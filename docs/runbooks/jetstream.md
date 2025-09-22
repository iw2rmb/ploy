# JetStream Control Plane Runbook

Operational checklist for deploying and maintaining the NATS JetStream control plane that backs the Consul KV migration roadmap.

## Overview
- **Job Spec**: `platform/nomad/jetstream.nomad.hcl`
- **Service Address**: `nats.ploy.local:4222` (TLS-terminated by Traefik TCP entrypoint)
- **Cluster Size**: 3 allocations scheduled on distinct Nomad clients
- **Auth Model**: Operator + system account JWTs stored in Nomad's variable store and rendered into the allocation at runtime

## Prerequisites
1. **Nomad Host Volume**: Define a host volume named `jetstream-data` on every client that will run the cluster. Example client config:
   ```hcl
   host_volume "jetstream-data" {
     path      = "/opt/ploy/jetstream"
     read_only = false
   }
   ```
2. **Nomad Variables**: Populate the following variable paths so the template renders credentials without baking secrets into HCL:
   ```bash
   nomad var put nats/operator jwt="$(cat operator.jwt)"
   nomad var put nats/system jwt="$(cat system.jwt)"
   nomad var put nats/system-creds creds="$(cat system.creds)"
   ```
   - Generate the artifacts with `nsc` (operator, system account, and creds bundle).
   - The values are single-line JWTs (`jwt`) and the multi-line creds file (`creds`).
   - Confirm with `nomad var get nats/operator` (output masked) before deployment.
3. **Traefik Dynamic Config**: Ensure the Traefik job from this repo is deployed so the `nats` TCP entrypoint (`:4222`) terminates TLS and forwards traffic to the JetStream allocation.
4. **CoreDNS Zone**: Apply the `iac/common/templates/coredns/zone.db.j2` rendered zone so `nats.ploy.local` resolves to the gateway IP.

## Deployment
Run all commands from the controller workstation; never apply changes directly on the VPS.

1. **Validate the job**
   ```bash
   nomad job validate platform/nomad/jetstream.nomad.hcl
   ```
2. **Plan the update via the job manager wrapper**
   ```bash
   /opt/hashicorp/bin/nomad-job-manager.sh plan \
     --job jetstream-cluster \
     --file platform/nomad/jetstream.nomad.hcl
   ```
3. **Apply** (the wrapper handles submit + monitor)
   ```bash
   /opt/hashicorp/bin/nomad-job-manager.sh run \
     --job jetstream-cluster \
     --file platform/nomad/jetstream.nomad.hcl
   ```
4. **Wait for allocations**
   ```bash
   /opt/hashicorp/bin/nomad-job-manager.sh wait --job jetstream-cluster --timeout 180
   ```

## Verification
1. **Health Endpoints**
   ```bash
   curl -fsS http://nats.ploy.local:8222/healthz
   curl -fsS http://nats.ploy.local:8222/jsz?consumers=true
   ```
2. **CLI Smoke Tests** (from controller shell with creds)
   ```bash
   nats account info --creds system.creds --server nats://nats.ploy.local:4222
   nats kv ls --creds system.creds --server nats://nats.ploy.local:4222
   ```
3. **Cluster Topology**
   ```bash
   /opt/hashicorp/bin/nomad-job-manager.sh logs --job jetstream-cluster --since "5m"
   ```
   Look for `JetStream cluster new stream leadership` events across all three allocations.
4. **DNS Check**
   ```bash
   resolvectl query nats.ploy.local
   ```
   Ensure the A record matches the CoreDNS gateway IP.

## Operations
- **Rolling Update**: Edit `platform/nomad/jetstream.nomad.hcl`, rerun the plan/run/wait sequence. The job's canary/rolling configuration updates one allocation at a time.
- **Credential Rotation**: Update the relevant Nomad variable(s), then restart the job via `nomad-job-manager.sh restart --job jetstream-cluster --alloc <alloc_id>` to pick up new secrets.
- **Storage Maintenance**: Host volume data lives under `/opt/ploy/jetstream`. Use per-node backups before destructive maintenance. Ensure free disk space for JetStream file limits (`64GiB` default in spec).
- **Traefik Integration**: TCP routing relies on the `traefik.tcp.routers.nats.*` tags within the job spec. Verify new Traefik deployments continue to include the `nats` entrypoint in `platform/nomad/traefik.hcl`.

## Troubleshooting
- **Allocation fails to start**: Inspect `/opt/hashicorp/bin/nomad-job-manager.sh logs --job jetstream-cluster --alloc <id> --both --lines 200`. Missing Nomad variables or host volume permissions are the usual root causes.
- **Cluster does not form quorum**: Confirm `nats.ploy.local` resolves to the active JetStream node and that port `6222` is reachable. The job manager logs include the advertised address per allocation.
- **Client connection refused**: Verify Traefik is running with the `nats` entrypoint and that `nats.ploy.local` resolves to the gateway. Falls back to `nomad alloc exec` with `nats bench` for internal connectivity tests.

Maintain this runbook alongside updates to the job spec, Traefik entrypoint, or credential workflow so operators have a single authoritative reference.
