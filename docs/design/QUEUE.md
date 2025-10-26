# Design Queue

 - [x] `docs/design/controlplane-mods-split/README.md` — Decompose the MODS HTTP handlers into cohesive files under 200 LOC.
 - [x] `docs/design/mods-service-test-split/README.md` — Split `internal/controlplane/mods/service_test.go` into focused test files and helpers.
 - [x] `docs/design/config-package-split/README.md` — Decompose `internal/api/config/config.go` into cohesive files.
 - [x] `docs/design/deploy-bootstrap-split/README.md` — Decompose `internal/deploy/bootstrap.go` into cohesive files.
 - [x] `docs/design/cli-ssh-artifacts/README.md` — Layer artifact upload/download atop the new SSH transport.
 - [x] `docs/design/scheduler-refactor/README.md` — Decompose the scheduler package monolith into focused files.
 - [x] `docs/design/controlplane-registry-refactor/README.md` — Split the registry HTTP handlers into focused files.
 - [x] `docs/design/gitlab-signer-refactor/README.md` — Split the GitLab signer into issuance, rotation, validation, and watcher files.
 - [x] `docs/design/controlplane-jobs-split/README.md` — Decompose the control-plane job handlers, streams, and DTOs into cohesive files.
 - [x] `docs/design/artifacts-cluster-client-split/README.md` — Decompose the artifacts cluster client file into cohesive files.
 - [x] `docs/design/sshtransport-manager-split/README.md` — Break `pkg/sshtransport/manager.go` into focused files for types, manager logic, and tunnel lifecycle.
 - [x] `docs/design/step-runner-split/README.md` — Decompose `internal/workflow/runtime/step/runner.go` into focused files for execution, streaming, specs, and interfaces.
