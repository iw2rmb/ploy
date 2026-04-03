[README.md](README.md) Runbook for MIG end-to-end scenarios, prerequisites, image publishing, and validation contracts.
[migs_e2e_test.go](migs_e2e_test.go) Go test suite covering MIG e2e scripts, spec guardrails, and strict local-cluster readiness checks.
[scenario-batch-run.sh](scenario-batch-run.sh) Scenario script exercising batch run creation, repo add/restart operations, status checks, and stop flow.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario validating Hydra mount permissions where `/in` is read-only and `/out` is writable.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario proving `/out` artifacts are uploaded and retrievable after run completion.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Multi-step scenario validating cross-node workspace rehydration and cumulative diff execution.
[scenario-multi-step/](scenario-multi-step) Multi-step migration scenario that verifies ordered step execution with gate and healing flow.
[scenario-orw-fail/](scenario-orw-fail) Failing ORW scenario that exercises router summary, healing, and re-gate success path.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Direct-Codex failing ORW scenario validating healing loop and prompt-required enforcement.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW scenario script asserting successful migration and run completion.
[scenario-post-mig-heal/](scenario-post-mig-heal) Multi-step scenario validating healing triggered by post-migration gate failures.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Prep lifecycle negative scenario verifying deterministic prep failure handling and no job fan-out.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Prep lifecycle happy-path scenario verifying readiness transition before downstream job creation.
[scenario-selftest.sh](scenario-selftest.sh) Harness self-check scenario validating container execution and followed-run log plumbing.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario validating stack-aware container image selection from spec image maps.
[scenario-tmpdir-blocked/](scenario-tmpdir-blocked) Scenario validating rejection of unsafe bundle entries such as traversal paths and symlinks.
[scenario-tmpdir-mixed/](scenario-tmpdir-mixed) Scenario validating mixed Hydra `in` inputs (file and script) inside container mounts.
[validate-hygiene.sh](validate-hygiene.sh) Script running repository hygiene gates (`make test`, `make vet`, and `make staticcheck`).
