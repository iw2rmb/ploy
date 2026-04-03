[README.md](README.md) Runbook for MIG e2e prerequisites, image publishing, scenario execution, and validation contracts.
[migs_e2e_test.go](migs_e2e_test.go) Go e2e suite that runs MIG scenarios and falls back to offline Hydra script/spec validation when cluster prerequisites are unavailable.
[scenario-batch-run.sh](scenario-batch-run.sh) End-to-end batch-run scenario that validates repo fan-out, job progression, and stop behavior.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario proving Hydra mount contract by rejecting writes to `/in` and allowing writes to `/out`.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario verifying files written under `/out` are uploaded and downloadable as run artifacts.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Multi-step scenario validating cross-node workspace rehydration and cumulative diff execution.
[scenario-multi-step/](scenario-multi-step) Multi-step Java migration scenario validating ordered steps with build-gate and healing flow.
[scenario-orw-fail/](scenario-orw-fail) Failing ORW scenario that asserts router output, healing execution, and re-gate success contracts.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex failing ORW scenario validating prompt-file enforcement and healing-loop recovery.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW scenario script that runs a successful migration and completion checks.
[scenario-post-mig-heal/](scenario-post-mig-heal) Post-migration gate-failure scenario that validates healing and re-gate progression in multi-step runs.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep lifecycle negative scenario verifying deterministic prep failure and blocked downstream job creation.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep lifecycle happy-path scenario verifying readiness transition before repo jobs start.
[scenario-selftest.sh](scenario-selftest.sh) Harness self-test script checking container execution and followed-run log plumbing.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario validating stack-aware image map resolution for migration and build-gate steps.
[scenario-tmpdir-blocked/](scenario-tmpdir-blocked) Scenario ensuring unsafe spec-bundle entries (traversal and symlink) are rejected deterministically.
[scenario-tmpdir-mixed/](scenario-tmpdir-mixed) Scenario validating Hydra `in` mounts for mixed fixture inputs used by the container.
[validate-hygiene.sh](validate-hygiene.sh) Script enforcing repository hygiene gates via `make test`, `make vet`, and `make staticcheck`.
