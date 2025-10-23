# Control-Plane Node Onboarding via API

## Context

The workstation CLI still performs sensitive deployment operations (worker onboarding, CA rotation,
GitLab credential management) by dialing etcd directly. Operators must expose the cluster’s etcd
instance through SSH tunnelling and set `PLOY_ETCD_ENDPOINTS`, even though the Ploy v2 API contract
documents `/v2/nodes` and related endpoints for these tasks. This tight coupling hinders security,
adds operator friction, and blocks multi-cluster workflows where the CLI should communicate only
through beacon DNS.

Bootstrap already records beacon metadata, API keys, and the cluster CA in the local descriptor, and
beacon DNS becomes resolvable immediately after bootstrap. We can therefore rely on HTTPS calls to
the control-plane API instead of direct etcd access.

## Goals

- Retire `PLOY_ETCD_ENDPOINTS` from workstation workflows.
- Make `ploy node add`, `ploy beacon rotate-ca`, and `ploy config gitlab` use control-plane REST
  endpoints secured by the deployment CA and bootstrap-issued API token.
- Preserve the recently introduced automatic ID generation (cluster ID, bootstrap node ID, worker
  IDs) and descriptor defaults.
- Keep bootstrap self-contained; after it completes, any follow-up CLI operation should require only
  the cached descriptor and beacon DNS.

## Non-goals

- Replacing etcd as the authoritative backing store for PKI/registry data.
- Offline or air-gapped onboarding without API reachability.
- Redesigning the PKI manager or worker registry internals.

## Proposed Architecture

### API Surface

1. **/v2/nodes (POST)**
   - Accept worker address, labels, and health probes.
   - Delegate certificate issuance and etcd writes to existing `deploy.RunWorkerJoin`.
   - Return worker ID, certificate metadata, probe results, and any generated credentials.
2. **/v2/nodes (GET/DELETE)**
   - Mirror existing registry functionality for listing and deregistering workers.
3. **/v2/beacon/rotate-ca (POST)**
   - Wrap the CA rotation manager to replace direct etcd calls.
4. **/v2/config/gitlab (GET/PUT)**
   - Provide an API-backed key/value store for GitLab signer configuration.

All endpoints require mutual TLS using the cluster CA and bearer tokens issued during bootstrap.

### CLI Client Layer

- Introduce a shared HTTPS client that loads the default descriptor (ID, beacon URL, CA path, API
  key) and performs authenticated requests.
- `ploy node add` constructs a POST `/v2/nodes` payload instead of opening an etcd client.
- `ploy beacon rotate-ca` and `ploy config gitlab` switch to the new endpoints.
- Detect `PLOY_ETCD_ENDPOINTS`; emit a deprecation warning and ignore it once API mode is the default.

### Bootstrap

- Continue generating cluster/node IDs and writing the descriptor locally.
- Confirm the descriptor contains everything the CLI needs: beacon URL, CA bundle path, API key, and
  default marker.
- Beacon DNS (`<node-id>.<cluster-id>.ploy`) is resolvable immediately after bootstrap; CLI requests
  use that host unless overridden.

## Migration Plan

1. **Phase 1 – Backend**
   - Implement the new REST handlers in the control plane, reusing deploy packages.
   - Add integration tests that validate etcd state changes through the API path.
2. **Phase 2 – CLI**
   - Ship an API client library; behind a feature flag, update `ploy node add`, `ploy beacon rotate-ca`,
     and `ploy config gitlab` to call the REST endpoints.
   - Provide a temporary fallback (`--use-etcd`) for emergency rollbacks.
3. **Phase 3 – Default Switch**
   - Flip the feature flag so API mode is standard.
   - Warn when `PLOY_ETCD_ENDPOINTS` is set but unused.
4. **Phase 4 – Cleanup**
   - Remove etcd client helpers and the environment variable from docs/envs/README.md.
   - Delete the fallback flag once telemetry confirms API usage is stable.

## Testing Strategy

- Unit tests for the new HTTP client (mutual TLS, auth, error handling).
- REST handler tests using `httptest.Server` and in-memory PKI/registry fakes.
- End-to-end lab run: bootstrap → `ploy node add` via API → verify worker joins cluster → CA rotation
  → GitLab config update.

## Risks & Mitigations

- **Bootstrap networking gaps**: if beacon DNS isn’t reachable from the operator machine, CLI falls
  back to an explicit `--beacon-url` override or `/etc/hosts` entry. Document this path.
- **API availability**: ensure control-plane handlers surface clear errors when dependencies (etcd,
  PKI manager) fail, so CLI can prompt retries rather than leaving the system in a partial state.
- **Long-running tunnels**: document that tunnelling is still required to reach the beacon endpoint
  in isolated networks, but `PLOY_ETCD_ENDPOINTS` is no longer part of the workflow.

## Future Work

- Expand `/v2/nodes` to stream probe output in real time.
- Layer role-based scopes onto API tokens (e.g., separate provisioning vs. read-only roles).
- Expose a dashboard view in the beacon UI powered by the same endpoints.

