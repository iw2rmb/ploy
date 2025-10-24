# Testing Expectations

While implementing Ploy v2, every task must ship with comprehensive unit and integration test
coverage.

- **Unit tests** — Cover control-plane logic, job lifecycle operations, queue transactions, and any
  new CLI functionality.
- **Integration/E2E tests** — Validate multi-node scenarios using the VPS lab described in
  [docs/next/vps-lab.md](vps-lab.md). Ensure real interactions with etcd, IPFS Cluster, and the queue
  behave as expected.
- **Timeouts** — All tests must include explicit timeouts or guard rails so they cannot hang
  indefinitely. Avoid unbounded waits on external resources; use context deadlines or test harness
  timeouts.
- **Automation** — Update CI pipelines to run the expanded test suites, failing fast on timeout or
  resource leakage.
- **Documentation** — Record any non-obvious setup steps or required environment variables in
  `docs/next/...` alongside the new features.

Failing to provide the required test coverage or guard rails should block a pull request until the
gap is addressed.
