[README.md](README.md) MIGS E2E runbook covering prerequisites, scenario usage, follow/log behavior, and healing/gate validation expectations.
[hydra_contract_offline_test.go](hydra_contract_offline_test.go) Offline contract tests for special-env Hydra rewrites, docs key coverage, and hydra scenario script conformance.
[migs_e2e_test.go](migs_e2e_test.go) E2E and offline-validation Go tests for codex entrypoint checks plus Hydra mount and out-upload scenario coverage.
[scenario-batch-run.sh](scenario-batch-run.sh) Batch-run scenario that creates a run, adds repos, inspects status, restarts a repo, and stops the batch.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario proving Hydra mount semantics by rejecting writes to /in and permitting writes to /out.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario validating that files written under /out are uploaded and retrievable from run artifacts.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Multi-step scenario validating cross-node workspace rehydration, ordered diff replay, and cumulative migration behavior.
[scenario-multi-step/](scenario-multi-step) Multi-step migration scenario that runs a spec-driven sequence with follow mode and artifact collection.
[scenario-orw-fail/](scenario-orw-fail) Failing ORW scenario asserting router output, healing execution, re-gate progression, and strict codex handshake contracts.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex failing ORW scenario validating prompt-file enforcement and healing-loop recovery without amata mode.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW Java upgrade scenario expecting Build Gate success and final successful run status.
[scenario-post-mig-heal/](scenario-post-mig-heal) Post-migration gate-failure scenario validating healing retries, re-gate success, and multi-step continuation.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep lifecycle failure scenario checking PrepFailed evidence and downstream repo-job gating behavior.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep lifecycle happy-path scenario checking PrepReady transition before repo jobs are enqueued.
[scenario-selftest.sh](scenario-selftest.sh) Minimal self-test scenario that validates run/follow execution and artifact plumbing on a simple container step.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario for stack-aware image resolution across exact-match, default-fallback, and missing-default error paths.
[scenario-bundle-blocked/](scenario-bundle-blocked) Scenario that uploads unsafe spec bundles and asserts deterministic rejection of traversal and symlink entries.
[scenario-in-mixed/](scenario-in-mixed) Scenario validating Hydra in mounts with mixed fixture inputs consumed from /in inside the container.
[validate-hygiene.sh](validate-hygiene.sh) Script that runs repository hygiene checks through test, vet, and staticcheck targets.
