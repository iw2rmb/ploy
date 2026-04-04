[README.md](README.md) MIGS E2E runbook with prerequisites, scenario invocation patterns, follow/log flow, and healing or gate expectations.
[hydra_contract_offline_test.go](hydra_contract_offline_test.go) Offline contract tests that verify Hydra rewrite semantics, docs key coverage, and scenario script conformance.
[migs_e2e_test.go](migs_e2e_test.go) End-to-end and offline validation tests for MIG scenarios including Hydra mount, out-upload, and mixed `/in` inputs.
[scenario-batch-run.sh](scenario-batch-run.sh) Batch-run scenario that creates a run, manages repos, checks status transitions, and exercises restart or stop flow.
[scenario-bundle-blocked/](scenario-bundle-blocked) Scenario that uploads unsafe spec bundles and asserts deterministic rejection of traversal and symlink entries.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario proving Hydra mount behavior by rejecting writes to `/in` while allowing writes to `/out`.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario validating that files written under `/out` are uploaded and retrievable from run artifacts.
[scenario-in-mixed/](scenario-in-mixed) Scenario validating mixed Hydra `in` mounts where a file and a directory are both materialized under `/in`.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Multi-step scenario validating cross-node workspace rehydration, ordered diff replay, and cumulative migration behavior.
[scenario-multi-step/](scenario-multi-step) Spec-driven multi-step migration scenario that runs sequenced steps with follow mode and artifact collection.
[scenario-orw-fail/](scenario-orw-fail) Failing ORW scenario asserting router output, healing execution, re-gate progression, and codex handshake contracts.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex failing ORW scenario validating prompt-file enforcement and healing-loop recovery without amata mode.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW Java upgrade scenario expecting Build Gate success and final successful run status.
[scenario-post-mig-heal/](scenario-post-mig-heal) Post-migration gate-failure scenario validating healing retries, re-gate success, and multi-step continuation.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep-lifecycle failure scenario checking PrepFailed evidence and downstream repo-job gating behavior.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep-lifecycle happy-path scenario checking PrepReady transition before repo jobs are enqueued.
[scenario-selftest.sh](scenario-selftest.sh) Minimal self-test scenario that validates run or follow execution and artifact plumbing on a simple step.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario for stack-aware image resolution across exact-match, default-fallback, and missing-default error paths.
[validate-hygiene.sh](validate-hygiene.sh) Script that runs repository hygiene checks through `go test`, `go vet`, and `staticcheck` targets.
