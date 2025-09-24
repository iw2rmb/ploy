# 02 Configurable Harness

- [x] Status: Done

## Why / What For
Even after dependency seams exist, tests still rely on fixed DNS names and credentials. A configurable harness ensures integration runs read endpoints and secrets from environment variables so they can execute both locally (with fakes) and inside the VPS Nomad job (with real services).

## Required Changes
- Replace hard-coded URLs (`seaweedfs-filer.storage.ploy.local`, `https://example/repo`, etc.) with values sourced from environment variables or dependency configuration.
- Provide sensible fallbacks/mocks for hermetic test runs when variables are absent.
- Document required variables in `docs/TESTING.md` and the design doc.

## Definition of Done
- Running integration tests with the appropriate env vars uses real services; missing vars cause tests to skip gracefully or use fakes.
- Configuration knobs are exposed via a single struct or package to simplify harness setup.
- Documentation lists mandatory variables and how to supply them.
Implementation details:
- Added `HarnessConfig` in `internal/mods/harness.go` to load controller and Seaweed endpoints from environment variables with sensible defaults.
- Replaced hard-coded SeaweedFS fallbacks with harness-driven candidates and exposed optional overrides via `MODS_SEAWEED_FALLBACKS` and `MODS_SEAWEED_MASTER`.
- Updated Mod integrations and service helpers to consume the harness configuration and allow hermetic test runs.


## Tests
- Execute `go test ./internal/mods -run TestJetstreamKBLockManager_*` to confirm unit coverage unaffected.
- In a harness with env vars set, run a focused integration subset (e.g., builder + SeaweedFS scenario) to validate the configuration is honoured.

## References
- [Design doc](../../../docs/design/mods-integration-tests/README.md)
- Depends on: [01-dependency-seams](01-dependency-seams.md)
