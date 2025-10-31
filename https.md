HTTPS Migration Plan (Ploy Next)

Objective
- Remove SSH tunnels for CLI/control-plane data paths and move to HTTPS everywhere.
- Expose a standard Docker Registry v2 surface for nodes and workstations.
- Add resilient multi-endpoint failover in the CLI with pinned CA trust from the cluster descriptor.

Endpoints
- Control plane API: https://api.<cluster-id>.ploy (primary), with /v2 routes (keep /v1 during transition).
- Registry v2: https://registry.<cluster-id>.ploy/v2/ (Docker client compatible).
- Optional Node API: https://<node-id>.<cluster-id>.ploy for direct node status (mTLS or token).

Certificates & Trust
- Generate a cluster CA; include its PEM in the workstation cluster descriptor and deploy to nodes.
- Server certs (SANs):
  - api.<cluster-id>.ploy + API VIP IPs
  - registry.<cluster-id>.ploy + Registry VIP IPs
  - <node-id>.<cluster-id>.ploy + node IP
- Docker daemon (workers): install CA at /etc/docker/certs.d/registry.<cluster-id>.ploy/ca.crt.

CLI Descriptor (proposed schema)
{
  "cluster_id": "alpha",
  "api_endpoints": ["https://203.0.113.10", "https://203.0.113.11"],
  "api_server_name": "api.alpha.ploy",
  "registry_host": "registry.alpha.ploy",
  "ca_bundle_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n"
}

CLI Behavior
- TLS transport uses ca_bundle_pem and sets tls.ServerName=api_server_name.
- Failover: try api_endpoints in order; on timeout/TLS/5xx, continue to next.
- Upload/report switch to HTTPS streaming (no SSH slots). Keep /v1 fallback behind a feature flag during rollout.
- Registry commands remain HTTP-based and can be pointed at /v2 (server now maps /v2 to internal handlers).

Server Changes
- TLS listeners/certs for api.<cluster-id>.ploy and registry.<cluster-id>.ploy.
- Add /v2 routes:
  - Registry: /v2/<repo>/manifests|blobs|tags (already mapped internally via handleRegistryV2).
  - Artifacts: POST /v2/artifacts/upload, GET /v2/artifacts/{id} (future slice; currently /v1 exists).
- Keep /v1 routes during transition; mark deprecated.

Job Images
- Use registry.<cluster-id>.ploy/ploy/<image>:<tag> in templates (Docker pulls over TLS).

Rollout
1) Stand up TLS (CA + certs), DNS for api.<cluster-id>.ploy and registry.<cluster-id>.ploy.
2) Enable /v2 registry alias (done), keep /v1/registry for CLI back-compat.
3) Update job templates’ registry host; publish images; verify node docker pulls.
4) Update CLI: add descriptor schema + HTTPS failover + switch upload/report to HTTPS.
5) Deprecate SSH slots and /v1 when telemetry shows no usage.

Status
- /v2 registry alias added: control-plane maps /v2/* to existing registry handlers (GET /v2/ returns 200).
- Implemented: descriptor-based HTTPS client with CA pinning, SNI, and multi-endpoint failover in CLI.
- Implemented: artifact upload/report over HTTPS (`/v2/artifacts/...`), SSH kept only as fallback for legacy flows.
