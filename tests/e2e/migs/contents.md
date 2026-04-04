[README.md](README.md) Runbook for MIG e2e prerequisites, image publishing, scenario execution, and validation contracts.
[hydra_contract_offline_test.go](hydra_contract_offline_test.go) Offline contract test validating Hydra env-to-mount mappings, docs coverage, and Hydra scenario script usage.
[migs_e2e_test.go](migs_e2e_test.go) Go e2e suite that gates live cluster runs, executes Hydra scenarios, and enforces mig spec prompt/mount contracts.
[scenario-batch-run.sh](scenario-batch-run.sh) Batch workflow scenario that creates a run, adds repos, checks status, restarts a repo, and stops the batch.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario proving Hydra mount policy by rejecting writes to `/in` and allowing writes to `/out`.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario verifying files written under `/out` are uploaded and retrievable as run artifacts.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Multi-step scenario validating cross-node workspace rehydration, ordered diff replay, and cumulative migration behavior.
[scenario-multi-step/](scenario-multi-step) Multi-step Java migration scenario validating ordered steps with build-gate and healing flow.
[scenario-orw-fail/](scenario-orw-fail) Failing ORW scenario that validates router output, healing execution, and successful re-gating.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex failing ORW scenario validating prompt-file enforcement and healing-loop recovery.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW scenario that runs Java upgrade migration and expects Build Gate success.
[scenario-post-mig-heal/](scenario-post-mig-heal) Post-migration gate-failure scenario validating healing retries and re-gate progression across steps.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep lifecycle failure scenario asserting `PrepFailed` evidence and that downstream repo jobs stay gated.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep lifecycle happy-path scenario asserting `PrepReady` transition before repo jobs are created.
[scenario-selftest.sh](scenario-selftest.sh) Container self-test scenario validating basic execution plus follow/log artifact plumbing.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario for stack-aware image resolution, including success and missing-default error-path specs.
[scenario-tmpdir-blocked/](scenario-tmpdir-blocked) Scenario ensuring unsafe spec-bundle entries (traversal and symlink) are rejected deterministically.
[scenario-tmpdir-mixed/](scenario-tmpdir-mixed) Scenario validating Hydra `in` mounts with mixed fixture inputs consumed inside the container.
[validate-hygiene.sh](validate-hygiene.sh) Script running repository hygiene gates via `make test`, `make vet`, and `make staticcheck`.
