[README.md](README.md) MIGS e2e runbook covering prerequisites, image publishing, scenario execution, and build-gate healing contracts.
[hydra_contract_offline_test.go](hydra_contract_offline_test.go) Offline Hydra contract tests validating env-key rewrites, docs coverage, and scenario script constraints.
[migs_e2e_test.go](migs_e2e_test.go) MIGS e2e harness that runs live cluster scenarios when available and otherwise executes inline offline contract checks.
[scenario-batch-run.sh](scenario-batch-run.sh) Batch-run workflow scenario that creates a run, adds repos, inspects status, and exercises restart or stop operations.
[scenario-bundle-blocked/](scenario-bundle-blocked) Scenario that submits unsafe bundles and asserts traversal or symlink inputs are rejected.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario validating Hydra mount rules by verifying `/in` is read-only and `/out` is writable.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario that verifies files written to `/out` are uploaded and retrievable as artifacts.
[scenario-in-mixed/](scenario-in-mixed) Scenario fixtures and runner validating mixed Hydra `/in` mounts with both file and directory inputs.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Three-step multi-node scenario validating ordered workspace diff replay, gate checks, and optional healing.
[scenario-multi-step/](scenario-multi-step) Multi-step migration scenario with router and healer prompts to validate sequenced gate-and-heal behavior.
[scenario-orw-fail/](scenario-orw-fail) ORW failure scenario using router plus healer flow to validate gate failure classification and recovery.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex ORW failure scenario validating prompt-file enforcement and healing-loop behavior without amata routing.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW Java upgrade scenario expecting successful gate execution and run completion.
[scenario-post-mig-heal/](scenario-post-mig-heal) Post-migration gate-failure scenario validating retryable healing and continuation across steps.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep lifecycle failure scenario asserting `PrepFailed` evidence and no downstream job creation.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep lifecycle success scenario asserting transition to `PrepReady` before repo jobs begin.
[scenario-selftest.sh](scenario-selftest.sh) Minimal runtime self-test scenario that validates follow logs and artifact plumbing on a simple container job.
[scenario-stack-aware-images/](scenario-stack-aware-images) Stack-aware image selection scenario covering exact stack resolution, default fallback behavior, and error-path fixtures.
[validate-hygiene.sh](validate-hygiene.sh) Hygiene script that runs repository-wide `make test`, `make vet`, and `make staticcheck` checks.
