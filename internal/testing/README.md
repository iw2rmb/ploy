# Testing Utilities

## Key Takeaways
- Collects reusable mocks, builders, assertions, and helpers so tests across API, CLI, Mods, and build code share a single toolkit.
- Provides light integration harnesses (e.g. VPS smoke runners, controller clients) that mirror production flows while remaining test-friendly.
- Keeps fixtures and fluent builders co-located, making it straightforward to spin up realistic entities without copy/pasting setup code.

## Feature Highlights
- **Custom Assertions** (`assertions/`) – Rich failure messages, convenience matchers, and retry-aware assertions tailored to Ploy services.
- **Builder DSLs** (`builders/`) – Fluent APIs for crafting Nomad jobs, storage objects, mods payloads, and more, reducing boilerplate in tests.
- **Mocks** (`mocks/`) – Drop-in doubles for storage, orchestration, Git providers, SeaweedFS, etc., wired to mimic production behaviour.
- **Fixtures** (`fixtures/`) – Golden files and static payloads used by regression tests (Dockerfiles, Nomad HCL, API responses).
- **Helpers** (`helpers/`) – Common utilities (HTTP clients, temporary repo scaffolding, log capture, diff comparison) shared by unit/integration suites.
- **Integration Harness** (`integration/`) – Thin wrappers that drive controller/VPS endpoints, used by E2E scripts and smoke tests.

## Package Map
- `assertions/` – `RetryAssert`, `RequireNoError`, JSON diff helpers, and comparator utilities.
- `builders/` – Fluent constructors for API requests, storage records, and mods workflows; includes randomised data helpers.
- `mocks/` – Mock implementations of core interfaces (storage.Storage, orchestration.KV, Git providers, build checkers).
- `fixtures/` – Golden assets (Nomad templates, Dockerfiles, JSON payloads) referenced by snapshot-style tests.
- `helpers/` – File/temp-dir utilities, HTTP client scaffolding, context/time overrides, coverage helpers.
- `integration/` – VPS/controller smoke-test harness (login, deploy, Nomad log fetch) reused by CLI/E2E suites.

## Usage Notes
- Prefer builders + mocks from this package instead of defining ad-hoc stubs—consistency keeps tests readable and reduces breakage during refactors.
- The integration helpers expect environment variables (e.g. `TARGET_HOST`, `PLOY_CONTROLLER`); check `tests/e2e` scripts for examples.
- Assertions expose retry loops for eventually-consistent systems; use them when validating asynchronous orchestration results.
- Adding new fixtures/builders? Include focused tests inside this package to document expected behaviour and prevent regressions.

## Related Docs
- `tests/` top-level suites exercise these utilities (unit, integration, E2E).
- `internal/build/README.md` and `internal/orchestration/README.md` describe modules that most mocks target.
- `internal/cli/README.md` explains CLI command tests that lean on these helpers.
