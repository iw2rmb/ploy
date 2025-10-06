# Build Gate ESLint Adapter

- [x] Completed (2025-09-29)

## Why / What For

Deliver JavaScript static analysis parity by wiring ESLint diagnostics into the
build gate registry, CLI summary, and checkpoint metadata so polyglot
repositories get consistent feedback alongside Go vet and Error Prone findings.

## Required Changes

- Implement an ESLint-backed static check adapter registered under the
  `javascript` language key with aliases for `js`/`node`/`javascript`.
- Support manifest options for lint targets, config file paths, and rule
  severity overrides (`rule_overrides`).
- Parse ESLint JSON output into `StaticCheckFailure` entries with normalized
  rule IDs, severities, and file locations.
- Update CLI workflow summary fixtures to include ESLint failure reporting.
- Refresh build gate design docs and design index with the new adapter scope and
  `docs/CHANGELOG.md` entry.

## Definition of Done

- ESLint adapter executes with configurable options (targets, config file, rule
  overrides) and produces `StaticCheckReport` entries consumed by the registry.
- Registry alias coverage ensures `js`, `node`, and `javascript` language keys
  reach the ESLint adapter; manifest overrides propagate options to execution.
- CLI summary renders ESLint failures alongside other static checks with
  accurate issue counts and severities.
- Documentation (`docs/design/build-gate/eslint/README.md`, design index, build
  gate design, SHIFT design) updated with status/verification.
- `go test ./...` passes with new adapter tests, and verification details
  recorded in changelog and roadmap log.

## Implementation Notes

- Default to invoking `eslint` from PATH; allow manifests/tests to override via
  adapter option or constructor helper.
- Always supply `--format json` and `--no-error-on-unmatched-pattern` to
  stabilise parsing and avoid fatal glob errors.
- Translate ESLint severity integers/fatal flags into registry severity levels
  (`error`, `warning`, `info`).
- Use the existing `commandRunner` abstraction to support deterministic unit
  tests.
- Capture stderr when JSON output is missing so operator errors surface clearly.

## Tests

- Adapter unit tests covering command invocation, option propagation
  (targets/config/rule overrides), JSON parsing (multi-message, fatal
  severities), and error paths.
- Registry alias/unit coverage ensuring language normalisation hits the ESLint
  adapter entry.
- CLI summary test updated with ESLint failure to validate output rendering.
- Full `go test ./...` run documented once GREEN.

## References

- Design record (`docs/design/build-gate/eslint/README.md`).
- Build gate architecture (`docs/design/build-gate/README.md`).
- Static check registry design
  (`docs/tasks/build-gate/03-static-check-registry.md`).
