# Internal code refactoring

## Summary

Several execution and client paths implement the same concern separately, or
retain machinery whose original responsibility no longer exists. These paths
can disagree about success, resource ownership, file resolution, and errors.

Consolidate the seven selected review findings into their existing owners.
Remove unused inputs and branches. Fix the behavior differences caused by the
duplicated paths. Keep all seven changes in this document, as requested, with
one independently verifiable delivery slice per finding.

## Scope

The owning worktree is `ploy`. The replacement boundaries are:

| Slice | Boundary | Result |
| --- | --- | --- |
| R1 | Node runtime initialization and `step.Runner.Run` | Missing runtime cannot produce successful execution |
| R2 | Spec compiler mount-source rebasing | `!include` and `ref` use the same file-kind and path rules |
| R3 | Per-job runtime setup and teardown | Gates and migrations release resources through one owner |
| R4 | CLI wave status, cancellation, and following | Shared response contract, HTTP commands, and polling |
| R5 | CLI server-sent event (SSE) reconnection | Each connection releases its resources before retry |
| R6 | Node snapshot hydration call boundary | Persistent workspace preparation has no unused inputs or temporary-resource wrapper |
| R7 | Claim service result and dependencies | One typed response and no unused event dependency |

Implementation includes affected callers and tests across the repository,
even when a caller is outside `internal/`. R4 also moves the existing wave
response declaration from the server handler to `internal/domain/api`.
Preserve JSON field names, timestamp formats, omitted fields, endpoint paths,
authentication, job ordering, artifact selection, and workspace retention.

The behavioral corrections are explicit: runtime configuration errors must
fail execution; included `tmp` sources must resolve correctly; wave follow
must accept the actual response and expose HTTP/cancellation failures; and
runtime/SSE resources must be released at their owning boundary. SSE caller
cancellation must also remain distinct from an idle timeout.

[Node cleanup](cleanup.md) owns admission, maintenance locks, disk pressure,
and whole-run retention. R3 closes process resources; it does not establish
host quiescence or move terminal reporting after artifact upload. R6 does not
authorize workspace deletion. The directory and cache contracts in
[Gradle cache design](gradle-cache-to-be.md) remain unchanged. These designs
are related boundaries, not implementation prerequisites for this DD.

Deferred work includes legacy recovery ordering and historical metadata
compatibility, TUI completion channels, run-list pagination, completion caches,
job-planning branches, remote-gate fields, and unrelated naming cleanup.
Do not add a general resource manager, polling framework, runtime simulator,
new endpoint, schema migration, or deployment change.

## Observed Constraint

### R1: An initialization failure can look like successful execution

[`initializeRuntime`](../internal/nodeagent/execution.go) logs a Docker client
construction error and supplies a nil runtime. [`Runner.Run`](../internal/workflow/step/runner.go)
then returns exit code zero without creating a container. This fallback can
make an unexecuted migration look successful.

The constructor configures the Docker client; it does not prove daemon
availability. The demonstrated boundary is a client configuration error, not
every Docker outage. [`TestRunner_Run`](../internal/workflow/step/runner_test.go)
currently expects success from a zero-value runner. No explicit simulator
option governs this production fallback.

### R2: Included temporary files resolve from the wrong directory

[`normalizeIncludedLocalPaths`](../internal/speccompiler/includes.go) lists
`in`, `out`, and `home`. [`normalizeStepLocalPaths`](../internal/speccompiler/refs.go)
also lists `tmp`. Their adapters separately parse mount entry strings.

For example, a root spec includes `fragments/step.yaml`:

```yaml
# spec.yaml
steps:
  - !include ./fragments/step.yaml
```

```yaml
# fragments/step.yaml; fragments/input.txt exists
image: docker.io/test/mig:latest
tmp:
  - ./input.txt:input.txt
```

The compiler currently looks for `input.txt` beside the root spec. The same
example with `in` works because that field is rebased. If both directories
contain a file with that name, the wrong file can be selected instead of an
error being reported.

### R3: Gate setup omits runtime teardown

[`executeGateJob`](../internal/nodeagent/gate_job.go) calls `initializeRuntime`
and defers only log-stream closure. Migration execution uses
[`initExecutionContext`](../internal/nodeagent/execution.go), whose cleanup
also closes the container runtime.

[`containerRuntime.Close`](../internal/workflow/step/container_docker_proxy.go)
closes the Docker client and proxies left by containers that never reach
`Wait`. Gate execution bypasses that cleanup. Initialization also leaves a
created runtime unclosed if later log-stream construction fails. These are
ownership gaps; the amount of retained resources has not been measured.

### R4: Wave commands disagree about the same HTTP contract

[`followMigWaveProject`](../internal/cli/mig/commands.go) and
[`commands_wave.go`](../internal/cli/app/commands_wave.go) separately implement
wave requests and terminal-state polling. The migration path omits HTTP status
checks and discards errors from its cancellation request.

It also strictly decodes into a struct containing only `status`, while
[`WaveSummary`](../internal/server/handlers/waves.go) includes `id`, `mig_id`,
and other fields. [`DecodeResponseJSON`](../internal/httpx/httpx.go) rejects
unknown fields, so a normal full response fails that partial decoder.
Both polling loops sleep without observing cancellation during the wait.

### R5: Reconnection retains completed connection contexts

[`Client.Stream`](../internal/cli/stream/client.go) calls `defer cancelConn()`
inside its retry loop. Each defer remains until the whole stream returns.
Repeated reconnections therefore retain child contexts beyond the connection
that used them. The loop also repeats the same backoff operation for read
failure and EOF, and mixes connection cleanup with retry decisions.
When idle timeout is enabled, the context-error branches label even parent
cancellation as an idle timeout. The connection boundary must distinguish
these causes while consolidating cleanup.

### R6: Hydration carries inputs it does not use

[`prepareStickyWorkspace`](../internal/nodeagent/workspace_hydration.go)
selects a `RepoMaterialization` from the manifest or reconstructs one from
the request. `downloadSnapshot` only checks that pointer for nil. The actual
download uses the run ID and verifies the request's commit SHA.

[`prepareStickyWorkspaceWithCleanup`](../internal/nodeagent/container_job.go)
then wraps the workspace in `tempResource` with an empty cleanup function.
The name suggests temporary ownership, although successive jobs and retained
resume need the same workspace.

### R7: Claim results and dependencies describe nonexistent branches

[`claimService`](../internal/server/handlers/nodes_claim_service.go) stores an
event service that it never reads. `claimResult.Response` is never assigned;
successful calls populate only `Payload`. The corresponding alternate HTTP
response branch is unreachable.

[`claimJobHandlerWithEvents`](../internal/server/handlers/nodes_claim.go)
therefore advertises a dependency that does not affect its behavior. The
claim-time terminal-error path explicitly constructs completion with nil event
dependencies; removing the unused field must not change that behavior.

## Target Contract

### R1: Execution requires a configured runtime

Runtime construction errors return through the existing job failure path.
`Runner.Run` rejects a nil `Containers` dependency before preparing mount
contents or executing other step work. A successful result requires an actual
runtime call and its successful result. Tests can supply an explicit fake
through the existing `ContainerRuntime` interface.

Replace the successful nil branch with an error. Do not add a fallback flag,
automatic simulator, or a new runtime-selection abstraction. Keep disabled
gate semantics and real container exit-code handling unchanged.

At the runner guard, a short comment can explain the consequence that the
signature cannot express:

```go
// Success advances the job chain, so an absent runtime must not simulate a run.
if r.Containers == nil {
    return Result{}, fmt.Errorf("container runtime is required")
}
```

### R2: One rebasing rule covers every Hydra file kind

[`contracts.HydraFileKinds`](../internal/workflow/contracts/hydra.go) remains
the field-list authority. Both YAML-node and map adapters iterate that list.
Retain their separate traversal because their input representations differ.
Both adapters call one entry-string function in `speccompiler`:

```go
func rebaseMountEntrySource(
    kind contracts.HydraFileKind,
    entry, sourcePath, outputBaseDir string,
) string
```

Replace the existing `normalizeMountEntrySource` name with this operation's
specific name. `HydraFileKind` replaces the `isHome` boolean. Rebase only the
source component. Preserve the destination, canonical hash entries, and the
`home` read-only suffix. Preserve existing treatment of absolute paths,
environment placeholders, home-relative paths, and malformed entries.
Parsing and path-safety validation retain their existing owners.

The declaring file determines a relative source's directory. Nested includes
and referenced steps must use the same rule. `ref` values still use their
existing reference-specific rebasing; they are not Hydra mount entries.

Put the rationale beside the shared rebasing function:

```go
// Relative sources belong to the file that declares them. Rebase only the
// source because the destination names a path inside the job container.
```

### R3: One initializer owns per-job runtime release

Keep `initExecutionContext` as the node's setup boundary. Move the construction
body into it and remove `initializeRuntime` after both callers migrate.
Retain the existing `executionContext` and returned cleanup function; do not
add a second lifetime wrapper.

On successful initialization, the caller receives the context and one cleanup
function. Gates and migrations defer that function immediately. On failure,
the initializer closes any resources it has already created. The production
container runtime's existing `Close` implementation remains the release
authority for its Docker client and proxies.

At job return, close the per-job runtime and flush the log streamer before taking
the final artifact bundle. Close local artifact log files before reading them
for upload. Preserve the existing upload-selection rules, status reporting,
and error classification. Cleanup errors remain diagnostic and must not hide
the original execution result. Job directory deletion remains in the outer
execution lifecycle.

Use the existing ownership boundary at the gate call site:

```go
execCtx, cleanup, err := r.initExecutionContext(ctx, req.RunID, req.JobID, req.ResumeCount)
if err != nil {
    // Existing failure reporting and artifact-selection path.
    return
}
defer cleanup()
```

The snippet shows call shape, not a replacement for the failure-reporting body.
Update the initializer's current comment, which mentions only log closure.
Keep the existing rationale on `containerRuntime.Close`; do not copy it into
every caller. At artifact finalization, explain the ordering:

```go
// Finalize log writers before bundling so the upload contains their last bytes.
```

### R4: Wave transport and polling have one owner

Move `handlers.WaveSummary` to `domainapi.WaveSummary` without changing its
fields or tags. Update server and client callers together; do not keep an
alias or a second status-only wire struct. The server keeps database-to-API
conversion in `waveToSummary`.

Add wave commands to the existing `internal/client` package. They follow the
same dependency pattern as `ListRunsCommand`: supplied HTTP client and base
URL, with transport through `httpx.DoJSON`. Both GET and cancellation require
HTTP 200 and decode the full shared response. Retain bounded decoding,
response closure, authentication, base-URL prefixes, and error-body handling.

The selected interfaces are:

```go
// GetWaveCommand and CancelWaveCommand each carry Client, BaseURL, and WaveID.
func (c GetWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error)
func (c CancelWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error)

type FollowWaveCommand struct {
    Get      GetWaveCommand
    OnUpdate func(domainapi.WaveSummary) error
}

func (c FollowWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error)
```

`FollowWaveCommand` fetches immediately, delivers each snapshot to `OnUpdate`,
then stops on `lifecycle.IsTerminalWaveStatus`. Otherwise it waits one second
with a context-aware timer. It returns request, decode, callback, or context
errors without adding transport retries. The callback owns rendering only;
it does not decide whether polling continues. Require a non-nil callback.

The command entry points retain presentation and policy:

| Entry point | Output and terminal policy |
| --- | --- |
| `wave status` | One full JSON summary; observing `Cancelled` is successful |
| `wave status --follow` | Full JSON summaries until `Finished` or `Cancelled` |
| `wave cancel` | Full JSON summary returned by cancellation; request failures are errors |
| `mig run --follow` | Existing `wave <id> <status>` lines; `Finished` succeeds and `Cancelled` returns an error |

Preserve successful JSON output fields, timestamp strings, omitted fields,
and existing formatting through output assertions. Do not reinterpret
`Finished` as proof that every child run succeeded. `wave runs` remains a
separate list operation; remove the status path from `fetchWaveJSON` and give
its remaining runs-only helper a specific name.

The migration entry point owns `--cap` and `--cancel-on-cap`. A local cap stops
watching and remains a successful return when no cancellation was requested.
If cancellation was requested, return success only after a successful cancel
response. Report cancellation failures. Distinguish local cap expiry from
parent cancellation or deadline; parent cancellation must not trigger an
automatic cancel request. Use the original caller context for the cancel
request, not the expired follow context or `context.Background()`. Keep the
existing HTTP-client timeout.

Two comments capture the non-obvious policy at its respective owners:

```go
// Finished means every child run is terminal; it does not imply all succeeded.
```

```go
// The local follow deadline has expired. Use the parent context so cancellation
// can complete while still respecting the caller's cancellation.
```

### R5: Each SSE connection has a bounded lifetime

Keep `Client.Stream` and its public API. Extract one private connection-attempt
function in the same package. That function owns the request context, response
body, and idle timer. Release all three before returning to the retry loop,
including connect failures, HTTP errors, EOF, read errors, and handler exit.
An idle callback must reference only its own connection context.

The attempt result distinguishes reconnect, graceful completion, and terminal
error. It also reports whether an event arrived and the latest valid event ID.
Use one explicit result instead of independent shared `gotDone`,
`connectionFailed`, and `handlerErr` state across the outer loop. The outer
loop owns the existing `reconnectState`, retry count, and retained event ID.
It makes one backoff call for a reconnect outcome.

Preserve these semantics:

- Connection/read failures and EOF may reconnect under the current retry limit.
- HTTP status failures and handler errors terminate the stream.
- `ErrDone` terminates successfully; a wrapped `ErrDone` remains recognized.
- A received event resets backoff and the retry count before reconnecting.
- Reconnection sends the latest valid `Last-Event-ID`.
- Idle timeout remains terminal. Correct the existing cancellation
  classification: return the parent context error when the caller cancels,
  rather than relabeling it as idle expiry.
- The existing event-size limit and unlimited-retry option remain unchanged.
- Server retry hints remain unsupported by the current `sse.Read` integration.

Use the existing backoff implementation. Do not add a retry engine or duplicate
terminal state in the TUI. A comment belongs beside the attempt-local defer:

```go
// A stream can reconnect indefinitely. Release this connection's context before
// backoff so completed attempts do not remain attached to the caller.
```

### R6: Snapshot preparation returns a persistent workspace path

Remove the manifest parameter from `prepareStickyWorkspace` and the repository
parameter from `downloadSnapshot`. Delete the selection/fallback loop,
`tempResource`, and `prepareStickyWorkspaceWithCleanup`. Both gate and
migration callers receive the path directly:

```go
func (r *runController) prepareStickyWorkspace(
    ctx context.Context, req StartRunRequest,
) (string, error)

func (r *runController) downloadSnapshot(
    ctx context.Context, req StartRunRequest, workspacePath string,
) error
```

Preserve sticky-workspace reuse, resume rejection when the retained workspace
is missing, chain-head-only reconstruction, partial-extraction cleanup, and
commit verification. Preserve snapshot routing, node authentication, and
archive path checks. The shared `RepoMaterialization` type remains because the
server-side Git fetcher uses it. Do not remove manifest fields as a side effect.

Replace the temporary-resource explanation with rationale on
`prepareStickyWorkspace`:

```go
// The workspace belongs to the run: successor jobs and retained resume reuse
// it. Per-job cleanup must not remove it.
```

### R7: Claims return their actual response type

Make the successful result type explicit:

```go
func (s *claimService) Claim(
    ctx context.Context, nodeID domaintypes.NodeID,
) (domainapi.NodeClaimResponse, error)
```

Remove `claimResult`, `Response any`, the unused event-service field and
variadic constructor argument, and the alternate response branch. Rename
`claimJobHandlerWithEvents` to `claimJobHandler` and update route registration
and tests. Preserve response JSON and the existing 200/204/404/500 mapping.

Keep deterministic claim-payload failure completion, unclaim behavior, and
their existing event behavior. Do not change the completion service or claim
transaction as part of this removal.

The types and handler name fully express this boundary. Add no replacement
`why` comment for the removed scaffolding. In particular, do not invent a
reason for the existing nil event dependency in terminal-error completion.

## Enforcement

| Contract | Enforcing owner | Competing machinery removed |
| --- | --- | --- |
| R1 | Runtime constructor error propagation and runner dependency guard | Successful nil-runtime execution |
| R2 | `HydraFileKinds` plus one source-rebasing function | Independent field lists and mount-entry parsing |
| R3 | `initExecutionContext` and its cleanup function | Separate gate teardown and nested initializer |
| R4 | `domainapi.WaveSummary`, shared client commands, existing lifecycle predicate | Handler-local wire type, partial status struct, duplicate request/poll loops |
| R5 | Attempt-local cleanup and one outer retry decision | Stream-lifetime connection defers and repeated backoff branches |
| R6 | Path-returning workspace preparation with request-owned snapshot identity | Unused hydration selection and no-op temporary-resource wrapper |
| R7 | Concrete claim return type and constructor dependencies | Alternate `any` response and unused event injection |

Compile-time caller updates enforce signature and DTO replacements. Behavioral
tests enforce failure, ownership, and timing guarantees that types cannot
express. Repository searches prove that the retired paths have no remaining
callers; searches alone do not establish correct behavior.

The proposed comments are implementation requirements only where the final
code still needs the stated rationale. Put them at the narrow owning symbol.
Remove superseded comments in the same slice. Do not copy the DD into source
headers, add comments to every helper, or link code to this temporary design.
Keep local rationale in code and operator-visible behavior in existing docs.

## Implementation Slice

R1 precedes R3 because they change the same initializer. The other slices are
independent. Keep the numbered review scope even if implementation schedules
R3 immediately after R1. A slice is complete only after all affected callers
use its selected owner; do not retain compatibility wrappers between slices.

For R3, R4, and R5, establish final signatures and one end-to-end path before
filling remaining behavior. If inactive stubs provide a useful green
checkpoint, mark them `DD:refactor Rn` and make them return explicit errors.
Never use empty success or the retired path as a stub fallback. Do not stub
deferred slices. Direct implementation is preferable for the smaller removals.

### R1 — Reject execution without a runtime

Update the initializer and runner together. Replace tests that rely on a nil
runtime with explicit fakes; retain a test that nil returns an error. Verify
that invalid Docker client configuration reaches terminal failure reporting
and cannot report success or advance the workspace baseline.

Healthy exit: every successful step has executed through a runtime. Run the
focused `internal/nodeagent` and `internal/workflow/step` tests. Gate teardown
remains deferred to R3; this slice changes no resource-retention policy.

### R2 — Share source rebasing

Implement the shared entry function and field iteration. Migrate both adapters
and remove their duplicate parsing. Extend existing compiler fixtures across
all Hydra kinds, nested includes, references, and `home :ro`. Include the
same-named-file case to prove that the declaring file selects the source.
Verify canonical hashes and invalid/path-escape inputs retain their contracts.

Healthy exit: include and reference forms resolve equivalent source files
under the same policy. Run `internal/speccompiler`, its CLI callers, and server
named-spec tests. Compiler pipeline consolidation is deferred.

### R3 — Close resources through one lifecycle

Merge initialization into `initExecutionContext`. Migrate gate execution and
remove its independent log close. Close the runtime if later initialization
fails. Preserve artifact finalization and upload ordering on all returns.

Use existing fake runtime/HTTP facilities to verify closure after success,
execution failure, cancellation, and setup failure. Cover a proxy that never
reaches `Wait`. Verify the final log bytes are available when bundling starts.

Healthy exit: gate and migration resources have the same release owner.
Run `internal/nodeagent` and `internal/workflow/step` tests, including race
checks for modified lifecycle behavior. Maintenance locks and terminal-status
ordering remain with `design/cleanup.md`.

### R4 — Share wave requests and following

Move the response type, implement the shared commands, and switch both CLI
entry points in one slice. Narrow the old generic wave-fetch helper to its
remaining runs-list responsibility. Reuse the terminal predicate and existing
HTTP helpers. Keep cap and presentation policy at the command boundary.

Verify a full server-shaped response, non-200 responses, invalid JSON, request
failure, cap expiry during requests and waits, failed cancellation, parent
cancellation, and both terminal statuses. Assert successful output contracts.

Healthy exit: both consumers use the same HTTP and polling owners. Run
`internal/client`, `internal/cli/mig`, `internal/cli/app`, domain API, and server
handler tests. Update the client package README and the wave-follow behavior
in `docs/runs.md`. Run-list transport and TUI follow are deferred.

### R5 — Bound each SSE connection attempt

Extract connection work and local cleanup, then reduce the outer loop to one
retry decision. Extend the existing stream tests rather than creating a
parallel retry test suite. Use a recording transport to assert that the prior
request context is cancelled and its body closed before the next attempt.

Verify graceful completion, read/connect failure, idle expiry, caller
cancellation, retry exhaustion, backoff reset, and event-ID resumption. Use
synchronization to test resource ordering instead of heap or timing guesses.

Healthy exit: completed attempts retain no connection resources while the
stream remains open. Run `internal/cli/stream` and its run/job/log consumers,
with race checks for timer and cancellation behavior. TUI completion channels
and server retry hints are deferred.

### R6 — Remove false temporary-workspace ownership

Change both signatures and callers together. Delete the unused input selection
and no-op wrapper. Replace wrapper-mechanics tests with retained-workspace
behavior assertions. Reuse the existing snapshot HTTP and archive fixtures.

Healthy exit: preparation returns the same persistent path without changing
reconstruction or cleanup conditions. Run node hydration, gate, and migration
tests. Remote-gate fields and manifest input removal are deferred.

### R7 — Simplify claim results

Change `Claim`, its constructor, handler, registration, and callers together.
Delete the unreachable branch and unused event dependency. Extend existing
claim tests only where response/error behavior needs proof.

Healthy exit: each successful claim returns `NodeClaimResponse` directly, and
all HTTP/error paths retain their behavior. Run server handler and route
contract tests. Claim admission and completion-service refactoring are deferred.

## Completion

Each slice must build, pass its focused checks, and leave one owner for the
replaced behavior. No retired implementation may remain as a fallback.
All seven slices are required for this DD to be complete.

After the final slice, run the repository-wide checks with isolated CLI config:

```sh
go build ./cmd/...
(
    ploy_refactor_config=$(mktemp -d) || exit 1
    trap 'rm -rf "$ploy_refactor_config"' EXIT
    PLOY_CONFIG_HOME="$ploy_refactor_config" go test ./...
)
go vet ./...
git diff --check
```

Also run the focused race checks required by R3 and R5. Report skipped Docker
or database integration cases separately from passing unit checks. The current
`make redundancy-check` references an absent `scripts/redundancy-check.sh`;
it cannot serve as proof of completion. Do not repair unrelated tooling in
this DD's implementation.

Search the full repository for retired symbols and branches: the nil-runtime
success path, `initializeRuntime`, duplicated mount-entry parsing,
`followMigWaveProject`, status handling in `fetchWaveJSON`, `tempResource`,
`prepareStickyWorkspaceWithCleanup`, `claimResult`, and
`claimJobHandlerWithEvents`. Confirm that server and client share the single
wave response declaration. Preserve unrelated uses of `RepoMaterialization`
and event services.

Check comments against final code and tests. Comments must explain a surviving
constraint, not the history of the refactor. Update existing current-state
documentation only when its behavior or ownership changes. As slices ship,
remove completed-only details from this DD while preserving prerequisites for
remaining work. Delete this DD when no unimplemented contract remains.

## References

- Runtime: [node setup](../internal/nodeagent/execution.go),
  [migration execution](../internal/nodeagent/container_job.go),
  [gate execution](../internal/nodeagent/gate_job.go),
  [runner](../internal/workflow/step/runner.go),
  [Docker runtime cleanup](../internal/workflow/step/container_docker_proxy.go).
- Compiler: [includes](../internal/speccompiler/includes.go),
  [references](../internal/speccompiler/refs.go),
  [file compilation](../internal/speccompiler/file_records.go),
  [Hydra contract](../internal/workflow/contracts/hydra.go).
- Waves: [server response](../internal/server/handlers/waves.go),
  [wave CLI](../internal/cli/app/commands_wave.go),
  [migration CLI](../internal/cli/mig/commands.go),
  [shared client pattern](../internal/client/lists.go),
  [HTTP helpers](../internal/httpx/httpx.go),
  [terminal predicate](../internal/workflow/lifecycle/status.go).
- Streaming: [SSE client](../internal/cli/stream/client.go),
  [existing tests](../internal/cli/stream/client_test.go).
- Hydration: [workspace preparation](../internal/nodeagent/workspace_hydration.go),
  [sticky-workspace tests](../internal/nodeagent/workspace_hydration_sticky_test.go).
- Claims: [service](../internal/server/handlers/nodes_claim_service.go),
  [HTTP handler](../internal/server/handlers/nodes_claim.go).
- Existing contracts: [runs and waves](../docs/runs.md),
  [lifecycle and runtime paths](../docs/migs-lifecycle.md),
  [CLI HTTP contracts](../docs/api/cli-http-client-contracts.md).
