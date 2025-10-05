# Certificate Metadata Migration

## What to Achieve

- Move ACME-issued and custom-uploaded certificate metadata from Consul +
  SeaweedFS into JetStream (Key-Value + Object Store) with a single,
  authoritative backend.
- Stream certificate bundles through the JetStream Object Store using chunked
  readers/writers so renewals avoid in-memory buffering spikes.
- Publish `certs.renewed` notifications with revision headers that downstream
  services (Traefik, app sidecars, CLI tools) consume to reload TLS assets
  without polling.

## Why It Matters

- Eliminates Consul KV and ad-hoc SeaweedFS paths from the certificate flow,
  aligning storage with the rest of the JetStream roadmap.
- Revision-aware metadata plus push notifications shorten the window between
  renewal and deployment, cutting user-facing TLS errors.
- Shared transport with the routing/env-store migrations means one credentials
  story, one observability surface, and simpler rotation tooling.

## Where Changes Will Affect

- `api/acme/storage.go`, `api/certificates/manager.go`, `api/acme/renewal.go` –
  persistence, renewal orchestration, and metadata models.
- `internal/nats/` client helpers plus a new certificate store module under
  `internal/certificates/` for Object Store interactions.
- Traefik sidecar + app runtime hooks (`platform/traefik/`, runtime env
  injectors) to subscribe to renewal events and fetch bundles.
- CLI + API surface (`internal/cli/domains/`, `api/domains/handler.go`)
  returning JetStream-derived metadata.
- Documentation (`docs/certificates.md`, `docs/FEATURES.md`,
  `roadmap/README.md`) capturing the new flow and operational guidance.

## Prerequisites

- JetStream cluster, connection pooling, and credential distribution completed
  from stages 01–04.
- Routing Object Store migration (stage 05) in place so controller bootstrap
  already provisions buckets/streams and exposes metrics.
- NATS credentials shared with Traefik/app sidecars and the controller (ensure
  TLS trust store includes NATS CA if listeners are TLS-only).

## Data Model & Event Contracts

- **Key-Value Bucket**: `certs.metadata`
  - History: 5; replicas: 3; max value size: 64 KiB.
  - Keys follow `domains/<domain>` and store JSON with fields:
    - `domain`, `app`, `provider` (`letsencrypt` or `custom`),
      `bundle_object` (`domains/example.com/<revision>.tar`).
    - Timestamps: `not_before`, `not_after`, `issued_at`, `renewed_at`.
    - Integrity: `fingerprint_sha256`, `serial_number`, `auto_renew` (bool).
  - Metadata headers: `X-Ploy-Revision` (uint64 revision),
    `X-Ploy-Trigger` (`acme` or `manual`), `X-Ploy-Writer` (service id) for
    audit trails.
- **Object Store Bucket**: `certs.bundle`
  - Objects stored per revision: `domains/<domain>/<revision>.tar`. Tarball
    contains `cert.pem`, `key.pem`, `issuer.pem`, `metadata.json` (checksum +
    timestamps).
  - Writers push via 128 KiB chunked streams; readers validate object size and
    SHA256 (stored in tar metadata) before writing to disk.
  - Retain the latest 3 objects per domain; garbage collect older revisions
    after successful subscriber acknowledgement.
- **Stream / Event Subject**: `certs.renewed`
  - JetStream stream name: `certs.events`, `MaxAckPending=64`,
    `Retention=Limits`.
  - Payload fields: `domain`, `revision`, `bundle`, `not_after`, `provider`.
    The bundle points at `domains/<domain>/<revision>.tar`.
  - Headers mirror KV metadata plus `Nats-Msg-Id = <domain>-<revision>` to
    dedupe replays. Durable consumer names: `traefik-gateway`, `app-runtime`.

## How to Implement

1. **Bootstrap JetStream Resources**
   - Extend controller bootstrap to create `certs.metadata`, `certs.bundle`, and
     `certs.events` if absent.
   - Apply consistent replication/retention settings, emit
     `ploy_certs_js_bootstrap_total` metric, and fail fast when provisioning
     fails.
2. **Introduce JetStream Certificate Store**
   - Add `internal/certificates/jetstream_store.go` implementing
     read/write/delete against the KV + Object Store pair.
   - Replace `api/acme/storage.go` usage of Consul + `storage.Storage` with the
     new store; remove SeaweedFS coupling.
   - Stream writes using `io.Pipe` + chunked writers to keep memory bounded
     (<512 KiB) for large chains.
3. **Metadata & Renewal Updates**
   - Update `api/certificates/manager.go` to persist the new metadata shape,
     including revision, bundle object path, and fingerprints.
   - Ensure provisioning and renewals both call the JetStream store and return
     revision identifiers to callers.
   - Remove Consul KV reads/writes and legacy fallback flags; JetStream becomes
     mandatory (no backward compatibility path).
4. **Event Publication**
   - After successful bundle upload + metadata write, publish on `certs.renewed`
     with headers referencing revision + checksum.
   - Include a small retry window (exponential backoff) if publish fails;
     surface metrics (`ploy_certs_events_published_total`,
     `ploy_certs_event_errors_total`).
5. **Consumer Wiring**
   - Traefik sidecar: add a lightweight subscriber that pulls events, downloads
     the referenced bundle, writes to `/opt/ploy/tls/<domain>/`, then triggers a
     Traefik dynamic config reload.
   - App runtime hooks: expose helper library for sidecars/agents to subscribe
     and refresh local cert caches (respect `X-Ploy-Revision` to avoid stale
     writes).
   - Persist last applied revision on disk
     (`/opt/ploy/state/certs/<domain>.rev`) for crash recovery.
6. **CLI & API Surface**
   - Update domain certificate endpoints and CLI commands to read from JetStream
     (include revision, fingerprint, next renewal timestamp).
   - Return helpful error states when the object store blob is missing; advise
     users via HTTP 502/503 along with support IDs.
7. **Backfill & Verification Utility**
   - Build `cmd/ploy-migrate-certs` to enumerate Consul keys + SeaweedFS files,
     upload them into JetStream, and compare fingerprints against the newly
     stored tarball.
   - Produce a manifest (`migrations/certs/<ts>.json`) listing migrated domains,
     revisions, and diff status; fail the run if mismatches occur.
8. **Cleanup & Follow-Up**
   - Remove Consul ACL/intentions tied to `ploy/certificates/*` and delete
     SeaweedFS certificate paths after successful verification.
   - Update docs/runbooks with new operational steps, including how to replay
     `certs.events` for troubleshooting.
   - Harden `docs/certificates.md` with the JetStream flow diagram and mention
     Traefik/app subscriber expectations.

## Observability & Operations

- Metrics: `ploy_certs_js_bootstrap_total`, `ploy_certs_bundle_write_bytes`,
  `ploy_certs_events_published_total`, `ploy_certs_consumer_lag_seconds`,
  `ploy_certs_reload_failures_total`.
- Logs: emit structured logs on bundle upload/retrieval with domain, revision,
  and checksum; mask private key content by design.
- Runbooks: add `docs/runbooks/certificate-metadata-migration.md` describing
  migration command, replaying events, and restoring from previous revisions.
- Troubleshooting: controller CLI command `ploy certs inspect <domain>`
  downloads a specific revision and prints metadata for support.
- Alerting: page when `consumers.ack_pending` exceeds threshold or when renewal
  events are older than 6h (`not_after - now < threshold`).

## Deliverables

- JetStream-backed certificate store module with supporting tests and metrics
  instrumentation.
- Updated ACME/renewal flows with event publication + retry logic.
- Traefik/app sidecar refresh hooks consuming `certs.renewed` and applying
  bundles atomically.
- Migration tool + manifest output and documentation updates
  (`docs/certificates.md`, `docs/FEATURES.md`, roadmap readme, new runbook).
- Removal of Consul + SeaweedFS certificate code paths and credentials.

## Expected Outcome

Certificate metadata and bundles live exclusively in JetStream, renewals trigger
immediate `certs.renewed` fan-out, and all consumers reload TLS assets without
polling or Consul dependencies.

## Tests

- **Unit**: JetStream store tests covering KV/Object Store interactions, chunked
  streaming, and metadata serialization; renewal manager tests asserting event
  emission + retries.
- **Integration**: In-memory JetStream harness exercising end-to-end
  issuance/renewal, verifying bundle retrieval and durable consumer behaviour
  (ack, replay, last revision persistence).
- **E2E**: Run ACME issuance and custom certificate upload in a staging
  environment, confirm Traefik reload logs (`certs.renewed` receipt) and CLI
  reflects new revisions; include failure injection (drop event ack) to validate
  replay flow.
