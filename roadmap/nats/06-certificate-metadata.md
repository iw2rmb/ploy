# Certificate Metadata Migration

## What to Achieve
Store ACME and custom certificate metadata in JetStream Key-Value/Object Store, and broadcast renewal events on `certs.renewed` for dependent services.

## Why It Matters
JetStream provides versioned metadata and efficient bundle distribution, enabling automated TLS reloads without bespoke Consul polling or manual hooks.

## Where Changes Will Affect
- `api/acme/storage.go`, `api/certificates/manager.go` – persistence layer, renewal workflows.
- TLS consumers (Traefik, app sidecars) – subscription to renewal topics or artifact fetch updates.
- Documentation (`docs/certificates.md`, `docs/FEATURES.md`) – outline the new rotation flow.

## How to Implement
1. Replace Consul KV interactions with JetStream KV/Object Store writes, maintaining revision metadata.
2. Stream certificate bundles using chunked readers to mirror the Object Store example’s memory profile.
3. Publish `certs.renewed` events containing domain + revision; listeners trigger reloads based on this notification.
4. Build a backfill utility to migrate existing metadata and confirm parity before switching over.
5. Update docs immediately after completion with the refreshed rotation sequence and troubleshooting steps.

## Expected Outcome
Certificates and renewals live in JetStream, and TLS consumers react to `certs.renewed` events automatically.

## Tests
- Unit: Extend certificate manager tests to exercise JetStream storage and renewal event emission.
- Integration: Run ACME renewal simulations to ensure Object Store uploads/downloads behave correctly.
- E2E: Perform a certificate issuance flow and verify Traefik reloads upon receiving the JetStream renewal event.
