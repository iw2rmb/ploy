# INFRASTRUCTURE.md — Minimal, Bullet‑Proof Bare Metal Setup

## Target
Highly available control plane + compute pools for FreeBSD/bhyve and a small Linux pool for Kontain/Firecracker.

## Hardware (minimum)
- **FreeBSD Control Plane (3 nodes)** — Nomad servers, Consul servers, Vault (HA), Harbor (or external), MinIO, HAProxy.
  - 8–16 cores, 64–128 GB RAM, 2 x NVMe (mirror), 2 x 10GbE.
- **FreeBSD Compute (3 nodes)** — bhyve unikernels + jails.
  - 16–32 cores, 128–256 GB RAM, 2 x NVMe (mirror), 2 x 25GbE.
- **Linux Compute (2 nodes)** — Firecracker/Kontain pool.
  - 16–32 cores, 128 GB RAM, NVMe, 25GbE.
- **Optional Storage (3 nodes)** — Ceph or MinIO dedicated.
  - 8–16 cores, 64–128 GB RAM, 8 x SSD + 2 x NVMe SLOG.

## Network
- Redundant top-of-rack (LACP), VLANs (mgmt, storage, workload).
- Anycast VIP for Ingress via VRRP (keepalived) or L2 solution.

## Step-by-Step Setup
1. **Install FreeBSD 14** on control & compute nodes (ZFS mirror).
2. **Enable bhyve**, install Nomad/Consul/Vault/HAProxy via pkg.
3. **Install WireGuard** for node mesh; configure peering between FreeBSD and Linux pools.
4. **Deploy Nomad/Consul/Vault** (HA): 3 servers each, TLS enabled.
5. **Deploy Harbor + ORAS** (or connect external registry). Create `ploy` project.
6. **Deploy MinIO** (or Ceph RGW). Configure buckets for logs/artifacts.
7. **Ingress**: HAProxy jail with ACME (certbot) hooks. Add wildcard `*.ployd.app` DNS to Ingress VIP.
8. **Linux pool**: Install KVM, Firecracker, Kontain runtime, Nomad client.
9. **Observability**: Prometheus, Grafana, Loki, OTEL collector on control plane; scrape Nomad/Consul and host exporters.
10. **OPA**: Load policies enforcing SBOM/signatures and SSH gating.
11. **Ploy Controller**: deploy controller service via Nomad; configure Harbor/Consul/Vault credentials.
12. **Test**: deploy `apps/go-hellosvc` (Lane A) and `apps/java-ordersvc` (Lane C/E). Validate `https://<sha>.go-hellosvc.ployd.app` works.

## Hardening
- Use mTLS for all internal services.
- Vault auto-unseal with HSM or cloud KMS.
- Separate admin and workload VLANs; ACLs for registry and secrets.
- Regular ZFS snapshots; off-site S3 replication for logs and SBOMs.
