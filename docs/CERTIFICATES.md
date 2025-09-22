# Certificate Management Architecture

This guide explains how the platform issues, stores, and renews TLS certificates for platform-managed subdomains and user-supplied custom domains after the JetStream migration.

## Architecture Overview

Ploy relies on two complementary certificate paths:

### 1. Traefik-Managed Platform Certificates (Infrastructure-Level)

**Purpose**: Automatic HTTPS for all platform and app subdomains routed through Traefik (for example `api.dev.ployman.app`, `myapp.dev.ployd.app`).  
**Scope**: Internal platform domains detected via Traefik tags.  
**Management**: Nomad system job rendered from `iac/common/templates/nomad-traefik-system.hcl.j2` and deployed through the HashiCorp playbook.

#### Characteristics
- ✅ Certificates are requested on-demand via Traefik's `default-acme` resolver (HTTP-01 with TLS-ALPN fallback).  
- ✅ Certificates live on each gateway node in `/opt/ploy/traefik-data/default-acme.json` and auto-renew ahead of expiry.  
- ✅ Contact email defaults to `admin@ployman.app` (override via `PLOY_PLATFORM_CERT_EMAIL`).  
- ✅ To exercise against Let's Encrypt staging, set `PLOY_ACME_CA=https://acme-staging-v02.api.letsencrypt.org/directory`.

#### Implementation Notes
- **Provisioning**: System job definition `platform/nomad/traefik.hcl` renders from the shared Ansible template.  
- **Configuration**: The `websecure` entrypoint enables TLS with `certresolver=default-acme`, so any TLS router triggers issuance.  
- **Persistence**: Host bind mounts keep `/opt/ploy/traefik-data` across container restarts; no JetStream involvement for Traefik ACME.

### 2. Custom Domain Certificates (API/CLI Path)

**Purpose**: Issue or ingest certificates for user-owned domains registered through `ploy domains:add`.  
**Scope**: Any domain mapped to an app, including platform wildcard fallbacks.  
**Management**: API (`api/certificates/*`, `api/acme/*`) and CLI (`internal/cli/domains/*`).

#### Characteristics (Post-Migration)
- ✅ Certificate bundles stream into JetStream rather than SeaweedFS. Metadata lives in a dedicated Key-Value bucket, and PEM assets are stored in the JetStream Object Store.  
- ✅ Each successful issuance or upload publishes a `certs.renewed` event. Traefik sidecars and app runtimes subscribe, download the referenced tarball, and reload TLS assets without polling.  
- ✅ Renewal service (`api/acme/renewal.go`) reads JetStream metadata and updates counters/timestamps after a refresh.  
- ✅ Platform wildcard certificates reuse the same JetStream persistence path, so a single storage backend covers all controller-issued certs.

## JetStream Certificate Store

| Component | Value | Notes |
|-----------|-------|-------|
| KV Bucket | `certs_metadata` | JSON metadata per domain (`domains/<domain>`). History=5, replicas configurable via `PLOY_CERTS_JETSTREAM_REPLICAS`. |
| Object Store | `certs_bundle` | Tarball per revision (`domains/<domain>/<timestamp>.tar`) containing `cert.pem`, `key.pem`, `issuer.pem`, `metadata.json`. Chunk size defaults to 128 KiB (`PLOY_CERTS_OBJECT_CHUNK_SIZE`). |
| Events Stream | `certs_events` | Publishes `certs.renewed` messages with headers `X-Ploy-Revision`, `X-Ploy-Bundle`, `X-Ploy-Digest`. Durable consumers include `traefik-gateway` and app runtime sync helpers. |

### Metadata Fields

The KV payload mirrors the `internal/certificates.Metadata` struct:

```json
{
  "domain": "custom-domain.com",
  "app": "myapp",
  "provider": "letsencrypt",
  "status": "active",
  "bundle_object": "domains/custom-domain.com/20240915T120301Z.tar",
  "not_before": "2024-09-15T11:58:02Z",
  "not_after": "2024-12-14T11:58:01Z",
  "auto_renew": true,
  "fingerprint_sha256": "...",
  "serial_number": "...",
  "revision": "17",
  "renewal_count": 2
}
```

Consumers persist the `bundle_object` and `revision` locally to guard against duplicate deliveries.

### Event Consumption

1. Durable JetStream consumer receives `certs.renewed` message.  
2. Sidecar downloads the referenced tarball, validates the SHA256 digest, and updates its local TLS directory (e.g., `/opt/ploy/tls/<domain>/`).  
3. After a successful reload, the consumer acknowledges the message. Missing acks trigger replay until the revision is applied.  
4. Operators can rebroadcast via `ploy certs inspect --rebroadcast <domain>` (planned CLI helper) or by calling `Store.Rebroadcast` directly.

## Operational Flow

1. **Issuance / Upload**: ACME handler or custom upload calls `Store.Save`, streaming PEM assets to the Object Store and writing metadata to KV.  
2. **Notification**: `certs.renewed` event fires with revision/bundle pointers.  
3. **Renewal Service**: Periodically queries `ExpiringSoon` to find domains within the renewal threshold (`PLOY_CERT_RENEWAL_THRESHOLD_DAYS`, default 30). Successful renewals increment `renewal_count`.  
4. **Consumers**: Traefik/app sidecars react to events; manual reloads are no longer required.  
5. **Deletion**: Removing a domain deletes KV metadata and the latest bundle object. Historical revisions can be garbage collected once consumers catch up.

## Environment Configuration

### Controller Runtime

```bash
export PLOY_CERTS_JETSTREAM_ENABLED=1
export PLOY_CERTS_JETSTREAM_URL="nats://nats.ploy.local:4222"
export PLOY_CERTS_METADATA_BUCKET=certs_metadata
export PLOY_CERTS_BUNDLE_BUCKET=certs_bundle
export PLOY_CERTS_EVENTS_STREAM=certs_events
export PLOY_CERTS_RENEWED_SUBJECT=certs.renewed
# Optional auth overrides
# export PLOY_CERTS_JETSTREAM_CREDS=/etc/ploy/nats/certs.creds
# export PLOY_CERTS_JETSTREAM_USER=ploy-cert
# export PLOY_CERTS_JETSTREAM_PASSWORD=...
```

Set these before deploying the controller so `initializeCertificateStore` can bootstrap the buckets/stream.

### Traefik/App Sidecars

```bash
export CERTS_JETSTREAM_URL="nats://nats.ploy.local:4222"
export CERTS_RENEWED_SUBJECT=certs.renewed
export CERTS_BUNDLE_BUCKET=certs_bundle
export CERTS_TLS_DIR=/opt/ploy/tls
```

Sidecars acknowledge only after cert reloads succeed; otherwise they NAK to request replay.

## Usage Examples

```bash
# Inspect controller view of a certificate
curl -sS "$PLOY_CONTROLLER/apps/myapp/certificates/custom-domain.com"

# Trigger manual renewal (for debugging)
curl -X POST "$PLOY_CONTROLLER/v1/certs/renew/custom-domain.com"

# Download the latest bundle tarball by revision
nats object get certs_bundle "domains/custom-domain.com/20240915T120301Z.tar" > bundle.tar
```

## Migration Utility (`cmd/ploy-migrate-certs`)

Use the migration tool before enabling JetStream in production:

```bash
# Dry-run migration (no writes)
cmd/ploy-migrate-certs --dry-run \
  --consul-addr=http://127.0.0.1:8500 \
  --legacy-prefix=ploy/certificates \
  --seaweed-filer=http://seaweedfs:8888 \
  --bundle-prefix=certificates/

# Execute migration into JetStream
cmd/ploy-migrate-certs \
  --consul-addr=http://127.0.0.1:8500 \
  --jetstream-url=nats://nats.ploy.local:4222 \
  --metadata-bucket=certs_metadata \
  --bundle-bucket=certs_bundle \
  --events-stream=certs_events
```

The tool:
- Reads legacy metadata from Consul (`ploy/certificates/apps/...`).  
- Streams PEM assets from SeaweedFS (or specified storage API).  
- Writes JetStream metadata/bundles via the same store used by the controller.  
- Emits a manifest (`migrations/certs/<timestamp>.json`) summarising migrated domains, detected diffs, and failures.

## Observability

- `certs_events` backlog exposes consumer lag; alert if `ack_pending` grows or events exceed 6h without ack.  
- Controller metrics (`ploy_certs_bundle_write_bytes`, `ploy_certs_events_published_total`, `ploy_certs_consumer_lag_seconds`) surface in `/metrics`.  
- Use `curl -sS "$PLOY_CONTROLLER/platform/api/logs?lines=200" | rg certs` for quick diagnostics.  
- `ploy certs inspect <domain>` (planned CLI) will fetch metadata and optionally rebroadcast revisions for troubleshoot scenarios.

## Key Principles

1. **JetStream is the source of truth** for ACME/custom certificate metadata and bundles. Consul/SeaweedFS paths are legacy-only.  
2. **Event-driven reloads** ensure Traefik and app sidecars update immediately on renewal.  
3. **Separation of concerns**: Traefik keeps its internal ACME store; controller-issued certificates rely on JetStream.  
4. **Secure distribution**: NATS credentials rotate via Vault; clients must trust the NATS CA before subscribing or fetching bundles.  
5. **Staging support**: Override `PLOY_CERT_STAGING=true` and `PLOY_CERT_RENEWAL_THRESHOLD_DAYS` during testing to avoid production rate limits.

## API Endpoints

```bash
# Domain certificates
curl -sS "$PLOY_CONTROLLER/apps/myapp/certificates"
curl -sS "$PLOY_CONTROLLER/apps/myapp/certificates/custom-domain.com"

# Certificate lifecycle (controller ACME endpoints)
curl -X POST "$PLOY_CONTROLLER/v1/certs/issue" -d '{"domains":["custom-domain.com"]}'
```

## Migration Notes

- Run `cmd/ploy-migrate-certs` in dry-run mode first to produce parity manifests.  
- Roll out sidecars/consumers before flipping the controller to ensure `certs.renewed` events are consumed.  
- After verifying JetStream, remove Consul ACLs for `ploy/certificates/*` and delete SeaweedFS `certificates/` objects.  
- Update runbooks referencing the legacy storage paths; this document is the source of truth for JetStream-based certificate workflows.
