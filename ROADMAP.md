# Typed Domain Model Hardening

Scope: Introduce domain-specific types and helpers to prevent mixing values (IDs, refs, CIDs, digests, durations, resources, protocols, log levels). Apply types in contracts, nodeagent, runtime, Mods API, CLI config, and selected server edges. Keep sqlc models unchanged; add adapters at boundaries. Fix the mislabeled container label.

Documentation: CHECKPOINT.md, PIPES.md, docs/api, internal/workflow/contracts/*, internal/nodeagent/*, internal/workflow/runtime/step/*, internal/mods/api/*, internal/cli/config/*, internal/server/config/*.

Legend: [ ] todo, [x] done.

## Foundation: Domain Types Package
- [x] Create `internal/domain/types` — Central, testable single source for new types.
  - Change: add files `ids.go`, `vcs.go`, `artifacts.go`, `duration.go`, `resources.go`, `network.go`, `logging.go`.
  - Test: unit tests per file covering JSON/Text marshal/unmarshal and validation.

- [x] IDs — `TicketID`, `RunID`, `StageID`, `StepID`, `ClusterID` with `String()` and `IsZero()`.
  - Change: `internal/domain/types/ids.go`.
  - Test: construct/compare; reject empty; ensure JSON roundtrip yields strings.

- [x] VCS — `RepoURL`, `GitRef`, `CommitSHA` with basic normalization.
  - Change: `internal/domain/types/vcs.go`.
  - Test: accept https/ssh/file; preserve value in JSON; trim spaces.

- [x] Artifacts — `CID`, `Sha256Digest` with `Validate()`; `Sha256Digest` enforces `sha256:`.
  - Change: `internal/domain/types/artifacts.go`.
  - Test: valid/invalid digests; CID non-empty; JSON roundtrip.

- [x] Duration — wrapper that marshals as duration string; parses via `time.ParseDuration`.
  - Change: `internal/domain/types/duration.go`.
  - Test: valid/invalid inputs; JSON/YAML strings map to `time.Duration`.

- [x] Resources — `CPUmilli`, `Bytes` with parsers for common units; helpers to Docker limits.
  - Change: `internal/domain/types/resources.go`.
  - Test: parse `500m`, `2`, `2Gi`, `10G`; overflow/invalid cases.

- [x] Network — `Protocol` enum (`tcp`, `udp`) with validation; string marshal.
  - Change: `internal/domain/types/network.go`.
  - Test: accept `tcp|udp`; reject others.

- [x] Logging — `LogLevel` enum (`debug|info|warn|error`) with validation.
  - Change: `internal/domain/types/logging.go`.
  - Test: accept known; reject unknown; JSON roundtrip.

 - [x] Labels — constants `LabelRunID`, `LabelStageID` and helpers.
  - Change: `internal/domain/types/labels.go` with `LabelsForRun(RunID)` and `LabelsForStep(StepID)`.
  - Test: map contains expected keys/values; empty input yields empty map.

- [x] UUID Bridge — helpers to convert domain IDs ↔ `pgtype.UUID`.
  - Change: `internal/domain/types/uuid.go`.
  - Test: roundtrip conversion; invalid UUID returns zero-value.

## Contracts: Apply Strong Types
 - [x] WorkflowTicket: `TicketID`, `RepoMaterialization.URL RepoURL`, `BaseRef/TargetRef GitRef`, `Commit CommitSHA`.
  - Change: `internal/workflow/contracts/workflow_ticket.go` structs + `Validate()` updates.
  - Test: existing tests updated; add cases for invalid URL scheme and missing target/commit.

- [ ] ManifestReference: introduce `type StageName string` used where applicable.
  - Change: `internal/workflow/contracts/manifest_reference.go` (types only; keep JSON fields).
  - Test: name/version non-empty; JSON stable.

- [ ] StepManifest: `ID StepID`, `Inputs[].SnapshotCID CID`, `Inputs[].DiffCID CID`, `StepInputArtifactRef.Digest Sha256Digest`, `StepResourceSpec` uses `CPUmilli`/`Bytes`, `StepRetentionSpec.TTL Duration`.
  - Change: `internal/workflow/contracts/step_manifest.go` type substitutions + validators use new methods.
  - Test: adapt existing manifest tests; add failure on bad digest/unit.

- [ ] WorkflowCheckpoint/Artifact: `TicketID`, `Stage StageName`, `CacheKey` stays string; artifacts reuse strong types.
  - Change: `internal/workflow/contracts/workflow_checkpoint.go`, `workflow_artifact.go`.
  - Test: validate stage/ticket typed values; artifact metadata required.

- [ ] BuildGate metadata: no field type changes; keep numeric; ensure compile stays green.
  - Change: none beyond imports if needed.
  - Test: existing.

## Node Agent: Requests and Execution
- [ ] StartRunRequest: swap to `RunID`, `RepoURL`, `GitRef`, `CommitSHA`; keep JSON as strings.
  - Change: `internal/nodeagent/handlers.go` struct; custom JSON (or TextMarshaler on types) ensures compatibility.
  - Test: handler request decode/encode; existing tests adjusted for types.

- [ ] buildManifestFromRequest: map typed fields to `StepManifest` with new types; normalize target ref default.
  - Change: `internal/nodeagent/execution.go` (helper function location as implemented in repo).
  - Test: `TestBuildManifestFromRequest` updated to assert typed values; existing behavior preserved.

- [ ] Runner request threading: add `TicketID` on `step.Request`; pass from controller to runner; used for labels.
  - Change: `internal/workflow/runtime/step/stub.go` (Request struct), `internal/nodeagent/execution.go` (call sites).
  - Test: runner unit test asserts label carries correct run id when runtime set.

## Runtime/Step: Limits and Labels
- [ ] ContainerSpec label fix: stop writing step ID into `com.ploy.run_id`; either thread `TicketID` or rename label.
  - Change: `internal/workflow/runtime/step/container_spec.go` — use `types.LabelRunID` and `req.TicketID`.
  - Test: unit test asserts label value equals run ID; no label when empty.

- [ ] Resource limits conversion: add `StepResourceSpec.ToLimits()` using `CPUmilli`/`Bytes`.
  - Change: `internal/workflow/contracts/step_manifest.go` (method) and `internal/workflow/runtime/step/container_docker.go` apply limits via returned values.
  - Test: runtime test covers limit application; zero means unlimited.

## Mods API: Type Hardening
- [ ] Ticket/Stage IDs: switch to `TicketID`, `StageID`, `JobID` where present; keep JSON strings.
  - Change: `internal/mods/api/types.go`.
  - Test: compile of handlers using these types; JSON roundtrip.

## CLI Config: ClusterID Type
- [ ] Introduce `ClusterID`; migrate `Descriptor.ClusterID` and helpers (`SaveDescriptor`, `SetDefault`, `LoadDefault`, `ListDescriptors`).
  - Change: `internal/cli/config/config.go`.
  - Test: update tests to use `ClusterID`; default marker logic unchanged.

## Manifests: Protocol Enum
- [ ] Replace free-form protocol strings with `Protocol` enum; validate on compile normalization.
  - Change: `internal/workflow/manifests/compilation.go` types for `ServicePort.Protocol` and `Edge.Protocols`.
  - Test: existing manifest tests updated; add rejection for invalid protocol.

## Server Config: Address Validation (Optional Type)
- [ ] Add `ValidateAddress(string) (netip.AddrPort, error)` and use in config validation.
  - Change: `internal/server/config/validate.go` plus helper in a new small file.
  - Test: good/bad addresses; defaults preserved.

## Logging: Event Level Guard
- [ ] Validate `Event.Level` at creation using `LogLevel` (map unknown to `info` or reject).
  - Change: `internal/server/events/service.go` before `CreateEvent`; normalize `params.Level`.
  - Test: service test persists only allowed levels; SSE stream uses normalized level.

## DB Boundary: UUID Adaptation (Non-invasive)
- [ ] Use UUID bridge in server/node edges where converting between domain IDs and `pgtype.UUID`.
  - Change: minimal wrappers in handlers that construct store params; no changes to sqlc.
  - Test: roundtrip of IDs through DB writes in handler tests stays green.

## Compatibility and Migrations
- [ ] JSON compatibility — ensure all new types marshal as original strings; no API break.
  - Change: rely on `encoding.TextMarshaler`/`TextUnmarshaler` on types.
  - Test: golden JSON fixtures for tickets, checkpoints, artifacts.

- [ ] Wire adapters — add helper funcs to convert strong types to existing usage where refactors are deferred.
  - Change: `internal/domain/types/*` small shims.
  - Test: compile-only plus basic unit tests.

## Documentation and Guard Rails
- [ ] Update docs for new type semantics and label fix.
  - Change: `CHECKPOINT.md`, `PIPES.md`, docs/api notes.
  - Test: `tests/guards/docs_guard_test.go` passes; links valid.

- [ ] Add lints: forbid hard-coded label keys and free-form protocols.
  - Change: small staticcheck or regex guard in tests (if guard suite exists).
  - Test: guard test trips on regressions.
