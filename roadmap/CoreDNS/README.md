# CoreDNS + etcd Migration Plan (Replace ConsulDNS)

This document proposes and tracks the migration of internal DNS resolution from Consul DNS to CoreDNS backed by etcd. The intent is to preserve existing service discovery ergonomics while improving operational control, performance, and plugin flexibility.

## Goals
- Replace Consul’s DNS server with CoreDNS using the `etcd` plugin as the authoritative store for service records.
- Preserve existing service name patterns and SRV/A record semantics for applications and platform services.
- Minimize downtime via phased, reversible cutover with dual-run/shadow and clear rollback.
- Improve observability (metrics/logs) and security (TLS, RBAC) for the DNS plane.

## Non‑Goals
- Removing Consul KV, health checks, or catalog from the platform entirely.
- Introducing a service mesh; this plan addresses DNS service discovery only.

## Current State (Summary)
- Consul provides: service catalog, health checks, and DNS (UDP/TCP :53) for zones like `service.consul` and/or `*.cluster.local`.
- Nomad jobs register in Consul; Traefik and internal services resolve via Consul DNS.
- Node-level resolvers forward queries to Consul (directly or via systemd-resolved/dnsmasq).

## Target Architecture
- CoreDNS runs as a highly available Nomad job (system or service) on 2–3 nodes.
- etcd runs as a 3‑node Nomad job with persistent volumes and TLS, serving as the authoritative backend for the CoreDNS `etcd` plugin.
- Applications and platform services resolve names via CoreDNS; write paths to service records go to etcd via:
  - native writers (preferred) from orchestrators/registrars, or
  - a temporary Consul→etcd sync bridge during migration.
- Upstream forwarding: CoreDNS forwards non‑authoritative queries to the cluster’s recursive resolvers.

## Naming and Data Model
- Zone: `cluster.local` (authoritative via CoreDNS + etcd). Additional zones may be added as needed.
- etcd layout follows the skydns convention used by CoreDNS:
  - Keys under `/skydns/<reversed-domain>/<service>/<instance>` store JSON records.
  - Example: service `api.default.svc.cluster.local` → `/skydns/local/cluster/svc/default/api/<instance>`.
- Record types supported: `A`, `AAAA`, `SRV`, `TXT` (per CoreDNS etcd plugin).

## Phased Plan

1) Discovery & Design
- Inventory zones, record types, and query patterns used against Consul DNS.
- Document required TTLs, negative caching behavior, and SRV usage.
- Define CoreDNS zones and etcd key mapping to preserve compatibility.

2) Provision etcd (HA)
- Deploy 3‑node etcd as a Nomad job with persistent volumes, client and peer TLS enabled.
- Lock down client access (firewall; mTLS certificates; least‑privilege).
- Expose health/metrics for SRE dashboards and alerts.

3) Deploy CoreDNS (HA)
- Deploy 2–3 replicas as a Nomad job (system preferred for node locality).
- Configure Corefile:
  - `etcd` plugin authoritative for `cluster.local`.
  - `forward` plugin for upstreams (e.g., `8.8.8.8`/`1.1.1.1` or on‑prem resolvers).
  - `cache`, `health`, `ready`, `errors`, `log`, `prometheus :9153`.
- Register CoreDNS service in Consul for monitoring only (no DNS served by Consul).

4) Data Population & Sync
- Implement a one‑time migrator to export current Consul service records to etcd keys.
- Stand up a temporary Consul→etcd sync bridge:
  - Watches Consul catalog/health.
  - Projects healthy service endpoints into etcd `skydns` keys.
  - Cleans up stale keys on deregistration.

5) Shadow Mode (Dual‑Run)
- Keep node resolvers pointed at Consul.
- Add CoreDNS as a secondary/conditional forwarder for the target zones on a subset of nodes.
- Compare query volume, latency, NXDOMAIN/ServFail rates between Consul and CoreDNS.

6) Controlled Cutover
- Update node resolvers to prefer CoreDNS for authoritative zones:
  - Option A: update systemd‑resolved/dnsmasq conditional forwarding.
  - Option B: point nodes directly to CoreDNS for all queries and let CoreDNS forward upstreams.
- Roll out gradually (canary nodes → a rack → full cluster).
- Monitor error budgets and revert quickly on regressions.

7) Decommission Consul DNS
- Once success criteria are met, disable Consul DNS listeners.
- Remove the Consul→etcd sync bridge only after all writers are native to etcd.

## Success Criteria
- ≥99.9% successful responses for authoritative zones during and after cutover.
- P50/P95 DNS latency equal to or better than baseline Consul DNS.
- No functional regressions in SRV/A lookups for platform services and apps.
- Clean rollback validated in staging prior to production cutover.

## Rollback Plan
- Re‑point node resolvers back to Consul DNS.
- Keep etcd and CoreDNS running for post‑mortem; revert traffic only.
- If needed, pause the sync bridge to avoid conflicting writes.

## Operations & Security
- etcd
  - Enable client/peer TLS, rotate certs periodically.
  - Restrict client access to CoreDNS and approved writers; firewall others.
  - Snapshot and backup etcd data (S3/SeaweedFS); document restore.
- CoreDNS
  - Expose `/metrics` on :9153 and scrape to monitoring.
  - Enable structured logging; forward to central logs.
  - Resource requests/limits adequate for peak QPS; enable `cache` plugin.

## Observability
- Metrics: CoreDNS (`coredns_*`), etcd (`etcd_*`), node resolver metrics.
- Logs: use platform Dev API shortcuts when debugging:
  - Controller logs: `curl -sS "$PLOY_CONTROLLER/platform/api/logs?lines=200"`
  - Traefik logs: `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"`
- Tests/e2e helper: `tests/e2e/deploy/fetch-logs.sh` (export `APP_NAME`, optional `LANE`, `SHA`, `LINES`, `TARGET_HOST`).

## Deployment Notes (Nomad/VPS)
- On VPS clusters, submit jobs via `/opt/hashicorp/bin/nomad-job-manager.sh` only; do not call raw `nomad`.
- Container images for `coredns` and `etcd` must be published to the internal Docker Registry and referenced by fully qualified names in job specs.
- Use bounded SSH operations and the job‑manager wrapper for logs and waits.

## Testing Strategy
- Local (unit):
  - Name resolution helpers (if any) and config generation code.
  - Serialization of etcd `skydns` records.
- VPS (E2E via Dev API):
  - E2E tests that deploy a sample app and validate SRV/A lookups through CoreDNS.
  - Example: `E2E_LOG_CONFIG=1 go test ./tests/e2e -tags e2e -v -run TestDNS_CoreDNS_Cutover -timeout 10m` with `PLOY_CONTROLLER` set to `https://api.dev.ployman.app/v1`.
- Coverage targets: ≥60% overall, ≥90% for critical DNS path utilities.

## Risks & Mitigations
- Record parity gaps (Consul vs skydns): map and test SRV/TXT semantics explicitly.
- Negative caching differences: tune CoreDNS `cache` TTLs and verify client behavior.
- Writer divergence during migration: keep the sync bridge authoritative until all writers are updated.
- Single‑point failures: run 3‑node etcd and ≥2 CoreDNS instances; use rolling updates.

## Timeline (Indicative)
- Week 1: Discovery, design, etcd/CoreDNS job specs and security review.
- Week 2: Deploy etcd/CoreDNS to staging; implement migrator and sync bridge.
- Week 3: Shadow mode, E2E tests, tune TTLs/forwarders.
- Week 4: Canary cutover → full cutover; decommission Consul DNS after bake‑in.

## Acceptance Checklist
- CoreDNS + etcd HA deployed with TLS and metrics.
- Migrator populated etcd; sync bridge live; parity verified.
- E2E tests pass against CoreDNS for all critical services.
- Cutover executed with no critical incidents; rollback validated.

## Examples

Corefile (excerpt):

```
cluster.local:53 {
    etcd {
        path /skydns
        endpoint https://etcd-0:2379 https://etcd-1:2379 https://etcd-2:2379
        tls cert /etc/certs/coredns.crt key /etc/certs/coredns.key ca /etc/certs/ca.crt
        upstream
    }
    cache 30
    errors
    log
    health
    ready
    prometheus :9153
}

. {
    forward . 1.1.1.1 8.8.8.8
    cache 30
    errors
}
```

etcd record (skydns JSON example):

```
{
  "host": "10.1.2.3",
  "port": 8080,
  "priority": 10,
  "weight": 100,
  "ttl": 30
}
```

Nomad Submission (operational note):
- Always use the job manager wrapper with bounded waits/log fetches, for example:
  - `/opt/hashicorp/bin/nomad-job-manager.sh submit --file coredns.hcl --wait --timeout 300s`
  - `/opt/hashicorp/bin/nomad-job-manager.sh logs --job coredns --lines 200`

## Ownership
- Platform: DNS/Networking team (primary), SRE (secondary), Application Platform (consulted).

## References
- CoreDNS plugins: https://coredns.io/plugins/
- etcd skydns format: https://coredns.io/plugins/etcd/
- Consul catalog: https://developer.hashicorp.com/consul

