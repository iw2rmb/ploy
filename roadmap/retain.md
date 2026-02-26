# Node Container Cleanup Under Disk Pressure

Scope: Implement `design/retain.md` end-to-end by removing retain-policy behavior and moving all container cleanup to a pre-claim disk-pressure path owned by node runtime. No backward compatibility.

Documentation: `design/retain.md`; `internal/nodeagent/claimer_loop.go`; `internal/nodeagent/claimer.go`; `internal/workflow/step/runner.go`; `internal/workflow/step/gate_docker.go`; `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/mods_spec_parse.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/contracts/parse_helpers.go`; `internal/workflow/contracts/step_manifest.go`; `internal/nodeagent/run_options.go`; `internal/nodeagent/manifest.go`; `cmd/ploy/mig_run_flags.go`; `cmd/ploy/mig_run_spec.go`; `cmd/ploy/mig_command.go`; `internal/domain/types/ids.go`; `docs/migs-lifecycle.md`; `docs/schemas/mig.example.yaml`; `docs/envs/README.md`; `cmd/ploy/README.md`

Legend: [ ] todo, [x] done.

## Phase 0: Behavior Contract Tests
- [x] Add/adjust tests that encode target behavior.
  - Repository: `ploy`
  - Component: `internal/workflow/step`, `internal/nodeagent`
  - Scope:
    - Add tests proving runner and gate executor do not delete containers on completion.
    - Add claim-loop tests for pre-claim guard behavior:
      - below threshold + cleanup succeeds => claim continues.
      - below threshold + cleanup exhausted => no claim HTTP call, loop backs off.
  - Snippets:
    - current runner path to delete: `if !req.Manifest.Retention.RetainContainer { _ = r.Containers.Remove(ctx, handle) }`
    - current claim entrypoint: `func (c *ClaimManager) claimAndExecute(ctx context.Context) (bool, error)`
  - Tests:
    - `go test ./internal/workflow/step -run 'Runner|Gate'`
    - `go test ./internal/nodeagent -run 'ClaimLoop|claimAndExecute'`

## Phase 1: Remove Retain Policy Surface
- [x] Remove `retain_container` from canonical contracts and parser.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`
  - Scope:
    - Remove `RetainContainer` from `ModStep`, `HealingSpec`, `RouterSpec`.
    - Remove retain parsing from `parseModLikeFields` and all parse paths that consume it.
    - Reject `retain_container` as forbidden input (explicit validation error), do not silently ignore.
  - Snippets:
    - parser entry today: `if v, ok := raw["retain_container"]; ok && v != nil { ... }`
    - target error style: `return nil, fmt.Errorf("%s.retain_container: forbidden", prefix)`
  - Tests:
    - update/add `internal/workflow/contracts/mods_spec_test.go` for forbidden-field validation.

- [x] Remove retain-dependent runtime option plumbing.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - Remove `RetainContainer` from `ModContainerSpec`.
    - Delete mapping in `modsSpecToRunOptions`.
    - Remove retain propagation in manifest builders.
  - Snippets:
    - current mapping: `RetainContainer: spec.BuildGate.Healing.RetainContainer`
  - Tests:
    - update `internal/nodeagent/run_options_test.go`
    - update `internal/nodeagent/claimer_spec_test.go`
    - update `internal/nodeagent/agent_manifest_builder_test.go`

- [x] Remove CLI retain switch and retain spec override behavior.
  - Repository: `ploy`
  - Component: `cmd/ploy`
  - Scope:
    - Delete `--retain-container` flag and help text.
    - Remove CLI logic writing `step0["retain_container"] = true`.
    - Update help fixtures and command docs.
  - Snippets:
    - current flag: `flags.Retain = fs.Bool("retain-container", false, "...")`
  - Tests:
    - update `cmd/ploy/mig_run_spec_test.go`
    - update `cmd/ploy/mig_run_spec_parsing_test.go`
    - update `cmd/ploy/help_flags_test.go` and `cmd/ploy/testdata/help_mig.txt`

## Phase 2: Stop Completion-Time Container Deletion
- [x] Remove immediate container deletion from step runner and gate executor.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope:
    - Remove post-wait `Remove` call from `Runner.Run`.
    - Remove best-effort `Remove` call from Docker gate executor.
    - Keep create/start/wait/log paths unchanged.
  - Snippets:
    - runner deletion block in `internal/workflow/step/runner.go`
    - gate deletion block in `internal/workflow/step/gate_docker.go`
  - Tests:
    - update `internal/workflow/step/gate_docker_test.go` to assert no remove call.
    - add/adjust runner tests to assert container lifecycle ends at `Wait`/`Logs`.

- [x] Remove retain-only manifest contract bits.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/nodeagent`, `internal/workflow/step`
  - Scope:
    - Remove `StepRetentionSpec` and `StepManifest.Retention` if no remaining consumer.
    - Drop `validateRetention`.
    - Remove `ContainerSpec.Retain` field if unused after deletion removal.
  - Snippets:
    - current type: `type StepRetentionSpec struct { RetainContainer bool; TTL types.Duration }`
  - Tests:
    - update `internal/workflow/contracts/step_manifest_test.go`
    - update `internal/workflow/step/container_spec*_test.go`

## Phase 3: Pre-Claim Docker-Root Disk Guard + FIFO Cleanup
- [x] Add a dedicated pre-claim cleanup service owned by node runtime.
  - Repository: `ploy`
  - Component: `internal/nodeagent` (new file, e.g. `claim_cleanup.go`)
  - Scope:
    - Implement a service called from claim path only.
    - Hardcode minimum free space threshold to `1 GiB` (`1 << 30`), no env knobs.
    - Read Docker root dir from Docker daemon `Info` (`DockerRootDir`).
    - Measure free bytes on Docker root filesystem (path-based fs usage for that mountpoint).
  - Snippets:
    - `info, _ := dockerClient.Info(ctx, client.InfoOptions{})`
    - `dockerRoot := strings.TrimSpace(info.Info.DockerRootDir)`
    - `const minDockerFreeBytes int64 = 1 << 30`
  - Tests:
    - new `internal/nodeagent/claim_cleanup_test.go` with fake docker/info/fs providers.

- [x] Implement FIFO deletion of stopped ploy-managed containers.
  - Repository: `ploy`
  - Component: `internal/nodeagent` pre-claim cleanup service
  - Scope:
    - Enumerate containers (all states), keep only:
      - ploy-managed labels: `com.ploy.run_id` or `com.ploy.job_id`.
      - non-running states (`state != "running"`).
    - Sort by `Created` ascending (oldest first).
    - Remove containers one-by-one, re-check free bytes after each removal.
    - Stop when free bytes reach threshold or eligible list is exhausted.
  - Snippets:
    - label constants: `types.LabelRunID`, `types.LabelJobID`
    - loop shape:
      - `for free < minDockerFreeBytes { removeOldestEligible(); recheckFree() }`
  - Tests:
    - assert filter correctness (managed + stopped only).
    - assert strict FIFO order on `Created` timestamps.
    - assert “exhausted list + still low disk” returns guard failure.

## Phase 4: Wire Guard Into Claim Loop Semantics
- [ ] Run cleanup guard immediately before every claim attempt.
  - Repository: `ploy`
  - Component: `internal/nodeagent/claimer_loop.go`, `internal/nodeagent/claimer.go`
  - Scope:
    - Call pre-claim guard at start of `claimAndExecute`, before `AcquireSlot`, before HTTP claim request.
    - If guard cannot restore `>= 1 GiB` free:
      - do not call `/v1/nodes/{id}/claim`.
      - return `(false, nil)` so existing backoff path handles retry.
    - Keep existing success/error/no-work backoff mechanics unchanged.
  - Snippets:
    - `ok, err := c.preClaimCleanup.EnsureCapacity(ctx)`
    - `if err != nil { return false, err }`
    - `if !ok { return false, nil }`
  - Tests:
    - extend `internal/nodeagent/claimer_loop_test.go` and `internal/nodeagent/agent_claim_test.go`:
      - guard fail => no claim request emitted.
      - guard pass => claim request proceeds.

- [ ] Add focused logging for cleanup decisions.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - Log free bytes before/after cleanup, removed container count, and skip-claim decision.
    - Keep logs concise and structured for ops debugging.
  - Snippets:
    - `slog.Warn("pre-claim disk guard blocked claim", "docker_root", root, "free_bytes", free, "threshold_bytes", minDockerFreeBytes)`
  - Tests:
    - optional log assertions in unit tests where logger capture exists; otherwise behavior-only tests.

## Phase 5: Docs and Contract Sync
- [ ] Update docs to remove retain policy language and document disk-pressure model.
  - Repository: `ploy`
  - Component: docs
  - Scope:
    - Update `docs/migs-lifecycle.md`, `docs/schemas/mig.example.yaml`, `docs/envs/README.md`, `cmd/ploy/README.md`.
    - Remove `retain_container` mentions from spec/help examples.
    - Add concise runtime behavior note: containers are retained by default, cleaned pre-claim under disk pressure.
  - Snippets:
    - “Cleanup trigger: before claim; threshold: 1 GiB on Docker data-root filesystem.”
  - Tests:
    - `go test ./docs/api/...` (where applicable)
    - `go test ./cmd/ploy/...` for help/spec docs tests.

- [ ] Preserve completion API contract (`POST /v1/jobs/{job_id}/complete` => `204`).
  - Repository: `ploy`
  - Component: server handlers + API docs
  - Scope:
    - Confirm no cleanup-decision payloads are added.
    - Keep endpoint and status code unchanged.
  - Snippets:
    - reference: `internal/server/handlers/jobs_complete.go`
  - Tests:
    - existing completion handler tests remain green; add one regression test if needed for empty response body.

## Phase 6: Verification
- [ ] Run targeted suites, then full validation.
  - Repository: `ploy`
  - Component: contracts, nodeagent, step runtime, CLI/docs
  - Scope:
    - Validate removed retain surfaces, new pre-claim guard, FIFO cleanup ordering, and claim-loop gating behavior.
  - Snippets:
    - `go test ./internal/workflow/contracts ./internal/workflow/step ./internal/nodeagent ./cmd/ploy/...`
    - `make test`
    - `make vet`
    - `make staticcheck`
  - Tests: all pass.

## Open Questions
- None.
