[README.md](README.md) Operator guide for running MIG end-to-end scenarios and interpreting expected outcomes.
[scenario-batch-run.sh](scenario-batch-run.sh) E2E script validating batch MIG submission and multi-repo run behavior.
[scenario-multi-node-rehydration/](scenario-multi-node-rehydration) Scenario verifying recovery rehydration behavior across multiple execution nodes.
[scenario-multi-step/](scenario-multi-step) Scenario validating ordered multi-step MIG execution and end-state correctness.
[scenario-orw-fail/](scenario-orw-fail) Failing build-gate scenario that exercises router summary, healing, and re-gate loop behavior.
[scenario-orw-fail-direct/](scenario-orw-fail-direct) Failing scenario using direct Codex mode to enforce prompt-driven router/healing execution.
[scenario-orw-pass.sh](scenario-orw-pass.sh) Happy-path ORW scenario script that validates successful rewrite and run completion.
[scenario-post-mig-heal/](scenario-post-mig-heal) Scenario validating healing flow triggered after post-mig gate failures.
[scenario-prep-fail.sh](scenario-prep-fail.sh) Negative scenario that validates preparation-stage failure handling and reporting.
[scenario-prep-ready.sh](scenario-prep-ready.sh) Setup scenario that prepares a repository branch for subsequent MIG e2e runs.
[scenario-selftest.sh](scenario-selftest.sh) Self-check scenario ensuring local e2e harness prerequisites are wired correctly.
[scenario-stack-aware-images/](scenario-stack-aware-images) Scenario suite validating stack-aware image selection and missing-default error behavior.
[scenario-hydra-mount-enforcement/](scenario-hydra-mount-enforcement) Scenario validating Hydra /in read-only and /out writable mount semantics.
[scenario-hydra-out-upload/](scenario-hydra-out-upload) Scenario validating /out write and artifact upload continuity end-to-end.
[scenario-tmpdir-blocked/](scenario-tmpdir-blocked) Scenario proving unsafe bundle entries (traversal, symlink) are rejected during Hydra materialization.
[scenario-tmpdir-mixed/](scenario-tmpdir-mixed) Scenario validating mixed Hydra in-record inputs (file and directory) are mounted under `/in`.
[validate-hygiene.sh](validate-hygiene.sh) Script that validates scenario scripts/specs for required hygiene and consistency rules.
