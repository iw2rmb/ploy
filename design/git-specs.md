# Git-Backed Named Specs

Status: desired state

## Summary

Named specs are discovered directly from Git repositories configured on the
Ploy server. Git is the authority for which named specs are currently
available. A separate `ploy spec push` publication step and a preloaded
database catalog are not used.

`ploy run <selector>` sends the selector to the server. The server refreshes
the configured spec repositories, resolves exactly one matching YAML file,
compiles it, stores the resulting immutable spec snapshot, and creates the run
against that snapshot.

Local submission remains supported. `ploy run /path/to/spec.yaml` compiles the
local file in the CLI and sends the canonical spec to the server as it does
today. Both local and server-side compilation use one shared compiler package.

## Scope

This design covers:

- server configuration through `PLOY_SPECS_REPOS`;
- shallow cloning and updating configured spec repositories;
- Git-backed discovery and listing of named specs;
- named-spec resolution during single-repository run submission;
- shared spec compilation for local and server-owned sources;
- named-run overrides;
- immutable run spec persistence and source attribution;
- removal of manual named-spec publication, version selection, and archiving.

This design changes the direct `ploy run` and `ploy spec` surfaces. Mig project
spec management remains separate.

## Why This Is Needed

The current named-spec workflow has two publication states:

1. the YAML committed to Git;
2. the named spec published to Ploy with `ploy spec push`.

A committed scenario is not available until it is published, and the stored
catalog can remain behind Git. Operators must manage an extra publication
command even though Git already contains the intended source and history.

Run submission already depends on Git to resolve and later hydrate the target
repository. Resolving a named spec from configured Git repositories does not
introduce a new class of dependency. The exact compiled spec is still persisted
before the run is created, so changes to a default branch cannot change an
existing run.

## Goals

- Make a commit on a configured repository's default branch sufficient to
  publish or remove a named spec.
- Keep Git as the only authority for the current named-spec catalog.
- Preserve local-path submission.
- Persist the exact compiled spec and source commit used by every run.
- Resolve a named selector and create its run in one server-owned operation.
- Use identical parsing, composition, validation, override, and bundle
  compilation logic for local and named specs.
- Keep the initial repository configuration intentionally small and static.
- Produce deterministic discovery, ambiguity errors, and list output.
- Prevent repository content from reading files outside its checkout when
  compilation runs inside `ployd`.

## Non-goals

- Configuring branches, tags, or commits per spec repository.
- Runtime mutation of the configured repository list.
- Webhooks or background synchronization.
- Historical named-spec selection with `@sha`.
- Named-spec archive and unarchive state.
- A database-backed catalog of currently available named specs.
- Cross-replica atomicity for the latest default-branch tips.
- Changing mig project spec storage or `ploy mig run`.
- Preserving the removed publication API or CLI contracts.

## Current Baseline (Observed)

- [`internal/cli/spec/named.go`](../internal/cli/spec/named.go) implements
  `ploy spec push`, Git-aware YAML discovery, publication, database-backed
  listing, archive actions, and SHA-qualified selectors.
- [`internal/cli/specpayload/mig_run_spec.go`](../internal/cli/specpayload/mig_run_spec.go)
  owns local spec parsing, `!include` and `ref` expansion, environment and local
  overlay processing, schema validation, file-record compilation, and bundle
  preparation.
- [`internal/cli/specpayload/mig_run_spec_bundle.go`](../internal/cli/specpayload/mig_run_spec_bundle.go)
  couples bundle compilation to the control-plane HTTP bundle endpoints.
- [`internal/cli/run/run_submit_spec.go`](../internal/cli/run/run_submit_spec.go)
  resolves a non-local argument through `GET /v1/specs/resolve` before the CLI
  submits `POST /v1/runs`.
- [`internal/cli/run/run_submit.go`](../internal/cli/run/run_submit.go) applies
  step environment and forced build-gate overrides in the CLI. A mutated named
  spec is submitted without its existing `spec_id`.
- [`internal/server/handlers/specs.go`](../internal/server/handlers/specs.go)
  publishes, lists, resolves, archives, and unarchives named rows in the
  `specs` table.
- [`internal/server/handlers/runs_submit.go`](../internal/server/handlers/runs_submit.go)
  requires canonical spec JSON in a direct run request, optionally accepts an
  existing `spec_id`, resolves the target repository commit, and creates the
  run's durable rows.
- [`internal/store/schema.sql`](../internal/store/schema.sql) stores named-spec
  source, SHA, timestamps, and archive state in the append-only `specs` table.
- [`internal/worker/hydration/git_fetcher.go`](../internal/worker/hydration/git_fetcher.go)
  provides commit-oriented node workspace hydration. Its Git authentication and
  command helpers are relevant, but its cache semantics do not refresh an
  existing checkout when no commit is supplied.

## Target Architecture

### Authority and persistence

The system has two distinct authorities:

- Configured Git default branches define the specs currently discoverable by
  name.
- The `specs` row referenced by a run defines the immutable spec used by that
  run.

Catalog discovery never changes an existing run. Removing or editing a YAML
file changes subsequent discovery only. Run status, restart, job creation,
snapshots, and artifact processing continue to read the stored spec referenced
by the run.

The catalog is not reconstructed from `specs` rows. Persisted rows are run
history, not publication state.

### Server configuration

`PLOY_SPECS_REPOS` is an optional comma-separated list of full Git repository
URLs:

```text
PLOY_SPECS_REPOS=https://gitlab.example.com/platform/migs.git,https://gitlab.example.com/team/scenarios.git
```

The server parses the variable once at startup.

Configuration rules:

- leading and trailing whitespace around the full value and each entry is
  ignored;
- an unset or whitespace-only value configures an empty catalog;
- empty entries in a non-empty value are invalid;
- every entry must be a valid supported repository URL;
- repository URLs are normalized before comparison;
- normalized duplicates are invalid;
- the remote default branch is always used;
- repository order does not define selector precedence;
- changing the list requires restarting the server.

The existing server Git authentication configuration is used. Credentials must
not be persisted in a checkout's `origin` URL or returned by list and error
responses.

An empty catalog does not prevent server startup or local-path runs. Named runs
and `ploy spec ls` fail with a clear "no spec repositories configured" error.

### Repository cache and refresh

The server owns a cache for configured spec repositories beneath its cache
root. The directory layout is internal and is not an API contract.

For every `ploy spec ls` request and every named run submission:

1. Refresh every configured repository from its remote default branch.
2. Resolve one full 40-character commit SHA for each repository.
3. Scan each repository at that fixed commit.
4. Build one request-local catalog snapshot.

The first refresh performs a shallow clone. Later refreshes fetch the current
default-branch tip with shallow history and update the cached checkout.

Refresh and scan rules:

- refreshes are serialized per repository;
- concurrent callers share an in-progress refresh instead of running competing
  Git mutations;
- scanning never observes a checkout while it is being updated;
- a request uses one fixed commit per repository for its complete resolution;
- repositories can refresh independently, but result ordering is deterministic;
- if any configured repository cannot be refreshed or scanned, the complete
  list or resolution operation fails;
- stale cached content is never silently treated as the current catalog after a
  refresh failure.

Different server replicas can briefly resolve different default-branch tips
when a branch moves between requests. This is acceptable because the selected
full SHA and compiled spec are persisted with each successful run.

### Discovery

Only tracked files whose names end in `.yaml` are candidates. Discovery uses
the Git index rather than an unrestricted filesystem walk.

A candidate is a named spec when its YAML document root is a mapping containing:

```yaml
apiVersion: ploy.mig/v1alpha1
name: non-empty-name
```

Only root keys participate in discovery. Nested `apiVersion` or `name` values
do not qualify a file. `description` is collected when it is a root scalar.

Each discovered entry contains:

- name;
- description;
- credential-free normalized repository URL;
- repository-relative YAML path;
- resolved full repository SHA.

Discovery does not publish or write database rows. Full composition, local-file
validation, bundle creation, and schema validation happen when a selected spec
is compiled for a run.

Catalog entries and list output are sorted by name, normalized repository URL,
and path.

### Selectors and ambiguity

Named runs continue to support:

- `<name>`;
- `<namespace/repo>:<name>`;
- `<domain>/<namespace/repo>:<name>`.

`@sha` suffixes are not supported.

Resolution applies the selector to the request-local catalog snapshot:

- zero matches return not found;
- one match proceeds to compilation;
- more than one match returns a conflict;
- ambiguity errors include every matching repository and YAML path.

Repository-qualified selectors disambiguate the same name across configured
repositories. They do not disambiguate two files with the same name inside the
same qualified repository; that remains a conflict.

### Shared compiler

Spec compilation moves from the CLI-owned `internal/cli/specpayload` domain to
a shared internal package. The package contains the format and transformation
logic, not command handling or server routing.

The shared compiler owns:

- YAML and JSON parsing;
- `!include` and `ref` composition;
- step selection;
- current-schema validation;
- local file-record validation and canonicalization;
- content-addressed bundle construction;
- step environment overrides;
- forced build-gate overrides;
- canonical JSON output.

The compiler accepts explicit dependencies for:

- source file access;
- environment lookup;
- optional configuration overlay;
- bundle lookup and persistence.

The two callers provide different adapters:

| Concern | Local CLI compilation | Server named compilation |
| --- | --- | --- |
| Source | User-selected local path | Selected repository and relative YAML path |
| File boundary | Existing local behavior | Strictly confined to repository root |
| Environment | Existing CLI environment behavior | No ambient `ployd` environment input |
| Local overlay | Existing `PLOY_CONFIG_HOME` overlay | Disabled |
| Bundle persistence | Control-plane HTTP adapter | Direct server bundle service adapter |

Server named compilation rejects absolute paths, `~/`, environment-expanded
paths, and any normalized or symlink-resolved path outside the selected
repository. Ambient server environment variables must not affect canonical
spec output. A named spec requiring an unavailable placeholder fails
compilation.

The server bundle adapter uses the existing content-addressed bundle storage
and deduplication behavior without making HTTP requests back to itself.

### CLI run behavior

The CLI preserves local-path precedence:

1. If the first argument exists as a local file or directory, compile it
   locally.
2. If it is explicitly local through `./`, `../`, or an absolute path, report a
   local path error when it cannot be loaded.
3. Otherwise treat it as a named selector.

For a local source, the CLI sends canonical `spec` JSON. For a named source, the
CLI sends `spec_selector` and does not resolve or download the spec first.

Local step selection through `<path>:<step-name>` remains a local compilation
feature.

### Run submission API

`POST /v1/runs` accepts exactly one spec source:

- `spec`: canonical JSON compiled by the CLI; or
- `spec_selector`: a named selector to resolve and compile on the server.

`spec_id` is not an input for direct run submission in the target contract.
The response continues to return the persisted `spec_id`.

Named-run overrides are sent with `spec_selector` in structured request fields:

- ordered `KEY=VALUE` entries grouped by step name;
- optional normalized forced build-gate values for `pre` and `post`.

The request shape is:

```json
{
  "spec_selector": "upgrade-java",
  "spec_overrides": {
    "step_envs": {
      "rewrite": ["JAVA_HOME=/opt/jdk-21", "MODE=strict"]
    },
    "build_gate_forced": {
      "pre": {
        "language": "java",
        "release": "21",
        "tool": "gradle"
      },
      "post": {
        "language": "java",
        "release": "21",
        "tool": "gradle"
      }
    }
  }
}
```

`step_envs` values preserve array order. Each forced build-gate phase requires
`language` and `release`; `tool` is optional.

The server applies those overrides through the shared compiler after resolving
and composing the selected YAML. CLI flag validation and precedence remain
unchanged: later step environment values win, the global forced build-gate flag
maps to both phases, and global and phase-specific forms are mutually exclusive.

Requests containing both `spec` and `spec_selector`, neither field, or
client-compiled `spec` together with named-spec overrides are invalid.

For a named request, the server performs operations in this order:

1. Validate the request and override shapes.
2. Refresh repositories and build a fixed catalog snapshot.
3. Resolve exactly one entry.
4. Compile the YAML at the resolved repository SHA.
5. Persist any required bundles.
6. Persist the canonical spec snapshot and its source attribution.
7. Create the mig, repository, wave, and run rows against that `spec_id`.

No run rows are created when refresh, resolution, or compilation fails.
Content-addressed bundles created before a later database failure can be
reclaimed through the existing unreferenced-bundle cleanup path.

The persisted source attribution includes the selector name, normalized
credential-free repository URL, repository-relative YAML path, and full source
SHA. Physical deduplication of identical snapshots is allowed but is not part
of selector or catalog semantics.

### Listing API and CLI

`GET /v1/specs` remains the endpoint used by `ploy spec ls`, but its result is
the live Git-backed catalog snapshot rather than rows from the `specs` table.

The list response exposes:

- name;
- description;
- source repository;
- repository-relative path;
- full source SHA.

`ploy spec ls` renders `NAME`, `SOURCE`, `PATH`, and the existing short display
form of `SHA`. The API always returns the full SHA.

The following surfaces are removed:

- `ploy spec push`;
- `ploy spec ls --archived`;
- `ploy spec <selector> --archive`;
- `ploy spec <selector> --unarchive`;
- `POST /v1/specs`;
- `GET /v1/specs/resolve`;
- `PATCH /v1/specs/{spec_id}`.

`ploy spec schema` and `ploy spec validate` remain.

### Failure contract

The server distinguishes these failures:

- no configured repositories: service unavailable for named listing or runs;
- repository refresh or scan failure: service unavailable, naming the
  credential-free repository;
- selector has no matches: not found;
- selector has multiple matches: conflict with repository and path choices;
- selected YAML cannot be composed, compiled, bundled, or validated: invalid
  run request with the selected repository, path, and SHA;
- run persistence failure: internal server error with no created run.

Secrets and credential-bearing clone URLs are never included in errors.

## Implementation Notes

### Package boundaries

- Extract format compilation from `internal/cli/specpayload` into a shared
  internal package.
- Keep CLI filesystem, environment, overlay, and HTTP bundle adapters under the
  CLI domain.
- Add a server-owned Git catalog service responsible for configuration,
  refresh, scanning, and selector resolution.
- Add a server bundle adapter over the existing blob persistence service.
- Keep node workspace hydration and server spec-repository refresh as separate
  domains. Share lower-level Git authentication or command utilities only where
  their contracts are identical.

### Storage

The `specs` table remains the immutable storage referenced by runs, waves, migs,
status, restart, and SBOM paths. Named-catalog queries, archive mutations, and
publication uniqueness constraints are no longer required for discovery.

Schema changes must retain the source attribution needed by historical runs.
Repository-relative YAML path must be represented in that attribution.

### API and generated surfaces

Update the domain DTOs, OpenAPI documents, handlers, CLI clients, and tests
together. The current contract is replaced directly; no legacy request-shape
guards or compatibility endpoint aliases are added.

## Milestones

### Milestone 1: Shared compiler

Scope:

- Extract compiler logic and dependency boundaries.
- Retain the current local-path run behavior through CLI adapters.
- Add strict repository-root file access support for the future server caller.

Expected results:

- CLI command packages no longer own core spec compilation.
- Local runs, step selection, overrides, includes, refs, file records, and
  bundle uploads remain behaviorally unchanged.

Testable outcome:

- Existing focused CLI and spec payload tests pass through the shared compiler.
- New traversal and symlink tests prove repository-root confinement.

### Milestone 2: Git-backed catalog and listing

Scope:

- Load and validate `PLOY_SPECS_REPOS`.
- Implement repository caching, refresh serialization, tracked-YAML discovery,
  deterministic snapshots, and selector matching.
- Change `GET /v1/specs` and `ploy spec ls` to use the live catalog.

Expected results:

- A committed YAML on a configured default branch appears without publication.
- Repository updates are visible on the next successful list request.

Testable outcome:

- Integration tests cover initial clone, unchanged refresh, changed default
  branch, duplicate names, qualified selectors, empty configuration, refresh
  failure, and concurrent list requests.

### Milestone 3: Server-resolved named runs

Scope:

- Add the mutually exclusive `spec` and `spec_selector` run request contract.
- Send named overrides to the server.
- Compile selected repository YAML through the shared compiler and direct
  bundle adapter.
- Persist source attribution and create the run against the immutable snapshot.

Expected results:

- Local-path runs remain client-compiled.
- Named runs no longer call a separate resolve endpoint or require preloaded
  database rows.
- Existing runs remain independent of later Git changes.

Testable outcome:

- End-to-end tests prove local submit, named submit, named overrides, source SHA
  attribution, ambiguity failure, invalid selected YAML, bundle materialization,
  and restart from the stored snapshot.

### Milestone 4: Remove publication state

Scope:

- Remove publication, archive, historical selector, database catalog, obsolete
  API, CLI, SQL, and documentation surfaces.
- Remove storage constraints used only by preloaded named-spec publication.

Expected results:

- Git-backed discovery is the only named-spec catalog.
- `ploy spec` contains only `schema`, `validate`, and `ls`.

Testable outcome:

- OpenAPI verification exposes only the target endpoints.
- Repository-wide tests and documentation link checks pass.
- Searches find no user-facing `spec push`, archive, or `@sha` named-selector
  contract.

## Acceptance Criteria

- `PLOY_SPECS_REPOS` configures zero or more full repository URLs and only their
  default branches.
- A valid committed named YAML is available to `ploy spec ls` and
  `ploy run <selector>` without another command.
- Every named lookup refreshes all configured repositories or fails without
  using stale data.
- Duplicate unqualified names fail deterministically and show all repository
  and path choices.
- Local `ploy run <path>` continues to compile and submit local specs.
- The CLI and server use the same core compiler.
- Server compilation cannot read outside the selected repository or depend on
  ambient `ployd` environment and local CLI configuration.
- Named overrides produce the same canonical result as equivalent local
  overrides.
- A successful named run stores canonical JSON, source repository, YAML path,
  and full source SHA before execution begins.
- Editing, deleting, or moving the source YAML after submission does not change
  run status, restart, jobs, artifacts, or SBOM behavior.
- `ploy spec push`, archive/unarchive, `@sha`, and the database-backed current
  catalog are absent.

## Risks

- Refreshing every configured repository increases named-run and list latency.
  Shallow fetches, persistent caches, and shared in-progress refreshes bound the
  cost without changing freshness semantics.
- One unavailable configured repository blocks the full lookup because absence
  and uniqueness cannot otherwise be proven.
- Server-side compilation expands the filesystem attack surface. Strict
  repository-root access and symlink tests are required before enabling named
  runs.
- Refactoring the compiler can accidentally change local-path behavior. The
  extraction must precede behavior changes and retain the existing tests.
- Multiple replicas can select adjacent default-branch commits during a branch
  update. Persisting the selected full SHA makes each individual run
  deterministic.
- Bundle persistence can succeed before run persistence fails. Existing
  content-addressing and unreferenced cleanup must remain effective.

## References

- [Runs and waves](../docs/runs.md)
- [Current CLI command reference](../cmd/ploy/README.md)
- [Current named-spec CLI implementation](../internal/cli/spec/named.go)
- [Current spec compiler](../internal/cli/specpayload/mig_run_spec.go)
- [Current run spec resolution](../internal/cli/run/run_submit_spec.go)
- [Current run submission handler](../internal/server/handlers/runs_submit.go)
- [Current named-spec handlers](../internal/server/handlers/specs.go)
- [Current specs schema](../internal/store/schema.sql)
- [Current Git hydration implementation](../internal/worker/hydration/git_fetcher.go)
