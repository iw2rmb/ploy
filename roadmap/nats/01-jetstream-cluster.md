# JetStream Control Plane Deployment

## What to Achieve

Stand up a highly available NATS JetStream cluster managed by Nomad and exposed
internally at `nats.ploy.local:<port>` for platform coordination traffic.

## Why It Matters

JetStream underpins the Consul KV migration by providing the storage and
messaging fabric for configuration, locks, and events. A reproducible Nomad
deployment ensures platform parity across environments and simplifies ongoing
operations.

## Where Changes Will Affect

- `platform/nomad/` – new Nomad job spec, allocation policies, service
  registration.
- `docs/FEATURES.md`, `docs/REPO.md`, `roadmap/README.md` – document the
  JetStream control plane availability.
- `docs/runbooks/` (or equivalent) – add bootstrap and credential management
  notes.

## How to Implement

1. Author a Nomad job using the Docker driver (or OCI image) with three
   JetStream servers, persistent volumes, and a Consul service check targeting
   `nats.ploy.local`.
2. Configure clustering (nats-server `cluster` and `jetstream` blocks) plus
   leaf/gateway placeholders for future expansion.
3. Set up authentication (NKeys/JWT) and export operator creds for internal
   clients.
4. Publish DNS or Traefik entry so clients resolve `nats.ploy.local` to the
   allocation front-end.
5. Validate the deployment with `nomad job plan` and `nomad job manager --wait`
   flows while tailing logs for cluster formation.
6. Capture bootstrap steps (accounts, creds, connection samples) in the
   documentation set immediately after the stage.

## Expected Outcome

A running Nomad-deployed JetStream cluster accessible at
`nats.ploy.local:<port>` with documented credentials, health checks, and
lifecycle procedures.

## Deliverables (2025-09-24)

- `platform/nomad/jetstream.nomad.hcl` job spec creating a three-instance
  JetStream cluster with host volumes, Nomad variable rendered credentials, and
  Traefik/Consul registration.
- Traefik system job updated to expose an optional TCP entrypoint on port 4222
  while internal clients connect directly to JetStream on
  `nats.ploy.local:4223`.
- CoreDNS zone (`iac/dev/vars/main.yml`) now publishes an A record for
  `nats.ploy.local` with an integration test in
  `tests/integration/dns/coredns_integration_test.go`.
- Runbook published at `docs/runbooks/jetstream.md` covering credential
  bootstrap, deployment, verification, and troubleshooting.

## Tests

- Unit: `nomad job validate platform/nomad/jetstream.nomad.hcl` (or equivalent)
  to catch spec errors.
- Integration: Run `nats account info` / `nats kv ls` against the cluster from
  the controller namespace to verify connectivity.
- E2E: Execute a smoke scenario (`nats req` + `nats kv add`) through the
  published address to confirm load balancing and persistence.
