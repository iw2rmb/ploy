# SSH-Only Bootstrap

## Why
Beacon HTTP plumbing (CA bundles, API tokens, resolver tweaks) lingers even though all control-plane calls now ride SSH tunnels. The extra TLS bootstrap steps contradict [`../ssh-descriptor-schema/README.md`](../ssh-descriptor-schema/README.md) and the unified onboarding flow in [`../cluster-add-onboarding/README.md`](../cluster-add-onboarding/README.md).

## What to do
1. Delete `cmd/ploy/beacon` commands plus `/v1/beacon/*` clients so the CLI never references HTTP beacon URLs.
2. Remove PKI bootstrap, CA rotation managers, resolver prompts, and workstation CA installers under `internal/deploy/bootstrap`.
3. Route every control-plane HTTP request through `pkg/sshtransport` using the descriptor-provided SSH metadata; drop legacy API-key headers and CA bundle reads.
4. Trim documentation and tests that still describe beacon URLs, CA bundles, or etcd tunnel bootstraps.
5. Dependencies: [`../ssh-descriptor-schema/README.md`](../ssh-descriptor-schema/README.md) (what data is available) and [`../cluster-add-onboarding/README.md`](../cluster-add-onboarding/README.md) (how nodes are installed).

## Where to change
- [`cmd/ploy/beacon`](../../../cmd/ploy/beacon) and [`internal/cli/beacon`](../../../internal/cli/beacon) packages for command/client removal.
- [`internal/deploy/bootstrap`](../../../internal/deploy/bootstrap) scripts and helpers that set up HTTP beacons or CA installers.
- [`pkg/sshtransport`](../../../pkg/sshtransport) to expose helpers for generic HTTP tunneling.
- [`docs/next/cli.md`](../../../docs/next/cli.md), [`docs/next/devops.md`](../../../docs/next/devops.md), and related tests describing beacon workflows.

## COSMIC evaluation
- **Complexity (2)** — removal of beacon commands plus rewire of HTTP clients.
- **Observability (1)** — existing SSH transport tests confirm proxying; removal validated via `go test ./...`.
- **Safety (1)** — work happens behind feature-flagged CLI release before general adoption.
- **Maintenance (0)** — codebase shrinks.
- **Integration (0)** — SSH transport already exists.
- **Confidence (0)** — unit tests plus manual smoke tests.
> Total: 4 (small slice).

## How to test
1. `go test ./cmd/ploy ./internal/cli ./pkg/sshtransport` after removal to ensure no beacon references remain.
2. `make build` and run `./dist/ploy cluster add` against the lab to confirm bootstrap works without CA prompts.
3. Tail control-plane logs to verify every HTTP call hits through the SSH tunnel rather than direct HTTPS.
