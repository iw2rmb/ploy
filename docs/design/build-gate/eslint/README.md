# Build Gate – ESLint Adapter (Roadmap 21)

## Status
- [x] Completed 2025-09-29 — ESLint adapter shipped with CLI summary coverage; see `docs/CHANGELOG.md` entry for test results.

## Purpose
Add a JavaScript static analysis adapter so the build gate stage enforces ESLint findings alongside Go vet and Error Prone coverage. This keeps CLI summaries and checkpoint metadata actionable for polyglot repositories that already depend on ESLint during CI.

## Background
- `../README.md` tracks the static check roadmap and still lists ESLint, Ruff, and Roslyn adapters as open follow-ups now that Error Prone shipped.
- `../../../../roadmap/build-gate/03-static-check-registry.md` defines the adapter registry contract and lane/manifest reconciliation flow used by existing adapters.
- `../../../../internal/workflow/buildgate/static_checks_registry.go` and `static_checks_types.go` provide the registry implementation and option plumbing we must plug into.
- `../../../../cmd/ploy/workflow_run_output_test.go` exercises the CLI build gate summary; it currently covers Go vet and Error Prone findings but lacks ESLint fixtures.
- Repositories describe ESLint expectations via manifest language entries (`javascript` aliases) but the registry cannot yet service them, leaving language parity incomplete.

## Goals
1. Implement an `ESLintAdapter` that shells out to `eslint` (default binary) while supporting manifest overrides for targets, config files, and rule severity adjustments.
2. Normalise ESLint diagnostics (rule ID, severity, file, line, column, message) into `StaticCheckFailure` entries so severity thresholds and CLI rendering behave consistently.
3. Extend CLI summary and build gate fixtures to include ESLint failures, ensuring users see JavaScript diagnostics alongside Go and Java results.
4. Document adapter scope, configuration options, and verification steps so future roadmap slices (Ruff, Roslyn) can reuse the pattern.

## Non-Goals
- Adding Ruff or Roslyn adapters (tracked separately).
- Implementing Grid/VPS remote execution for ESLint (workstation sandbox only).
- Managing package installation or Node version resolution (assume ESLint binary available via PATH or manifest-provided wrapper).

## Proposed Changes
1. Create `internal/workflow/buildgate/eslint_adapter.go` matching the existing adapter abstraction with command runner injection and metadata exposure.
2. Parse ESLint JSON output (`eslint --format json`) to collect messages, map severities (`0/1/2` + `fatal`) to registry severities, and aggregate into failures.
3. Support manifest options:
   - `targets`: directories/files list (default `.`) separated by commas, spaces, or newlines.
   - `config`: relative path to an ESLint configuration file (passed via `--config`).
   - `rule_overrides`: newline/comma-separated `rule:severity` pairs rendered as repeated `--rule` flags.
4. Update `normalizeLanguage` aliases if necessary to keep `js`, `node`, and `javascript` pointing at the adapter’s canonical key.
5. Add unit tests covering option propagation, command invocation, severity mapping, default targets, JSON parsing (including multiple messages per file), and command error behaviour when diagnostics are absent.
6. Extend CLI workflow summary tests/fixtures to assert ESLint findings appear with language alias `javascript`.

## Implementation Plan
- **Adapter construction**: Reuse `commandRunner` with defaults mirroring other adapters. Provide `WithESLintEnv` and `WithESLintBinary` options plus an unexported `withESLintCommandRunner` for tests.
- **Argument assembly**: Always pass `--format json`, `--no-error-on-unmatched-pattern`, optional `--config`, repeated `--rule`, and targets (default `.`). Merge `req.Options` into the run.
- **Parsing**: Decode JSON into lightweight structs and convert to `StaticCheckFailure` entries. Treat numeric severities `2` or `fatal` as `error`, `1` as `warning`, and others as `info`.
- **Registry wiring**: Register adapter metadata with language `javascript`, tool `ESLint`, default severity `error`; ensure alias coverage via tests if any new cases added.
- **CLI summary**: Update fixture to include a failing ESLint report so summary output enumerates it alongside other adapters.
- **Docs/Roadmap**: Record scope/deliverables in roadmap task `../../../../roadmap/build-gate/09-eslint-adapter.md`, update design index, and refresh build gate design follow-ups.

## Dependencies
- `../README.md`
- `../../../../roadmap/build-gate/09-eslint-adapter.md`
- `../../../../roadmap/build-gate/03-static-check-registry.md`
- `../../../../internal/workflow/buildgate/static_checks_registry.go`
- `../../../../internal/workflow/buildgate/static_checks_types.go`
- `../../../../cmd/ploy/workflow_run_output_test.go`

## Risks & Mitigations
- **Formatter drift**: ESLint JSON schema may evolve; mitigate by unmarshalling into minimal structs, ignoring unknown fields, and covering multi-message fixtures.
- **Binary availability**: Workstation may lack a global `eslint`; allow manifest overrides via `req.Options["binary"]` or `WithESLintBinary` for CI wrappers, and surface explicit execution errors.
- **Config mismatch**: Relative config paths may not resolve if manifests provide directories; document expectation for repo-root-relative paths and rely on user-supplied options.
- **Severity interpretation**: ESLint warns vs errors based on configuration; tests will ensure severity thresholds respect registry expectations.

## Test Strategy
- Targeted unit tests for the adapter (`internal/workflow/buildgate/eslint_adapter_test.go`) covering option propagation, JSON parsing, and failure severity mapping.
- Registry alias test ensuring `js`/`node`/`javascript` map to the ESLint adapter key.
- CLI summary test update verifying ESLint output renders correctly.
- Repository-wide `go test ./...` prior to marking the slice complete; record command/date in verification logs.

## Deliverables
- `internal/workflow/buildgate/eslint_adapter.go` and accompanying tests.
- Updated CLI fixtures and any registry alias tests.
- Roadmap task `../../../../roadmap/build-gate/09-eslint-adapter.md` describing scope, DOD, and verification.
- Documentation updates: this design record, design index entry, build gate design follow-ups, SHIFT design next steps, and changelog entry with date/verification details.
