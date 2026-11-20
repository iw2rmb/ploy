# Ploy SBOM JVM version lookup

## Summary

Ploy stores dependency versions and gate JVM metadata but has no cross-run
version lookup. Add `GET /v1/sbom/jvm/{jvm}/versions?for=&with=`.

## Scope

Requires [Flexible JVM dependency constraints](jvm-version-selectors.md).
Reuse its selectors and response contract.

Search existing storage at gate-job granularity. One job can contain packages
from multiple modules. The user selected this observation boundary for the
existing stored data.

Module-preserving ingestion, source federation, caching, retention changes, and
automatic version relaxation remain outside this DD. No schema migration or
new index is included.

## Observed Constraint

- `sboms` stores `(job_id, repo_id, lib, ver)` without module identity.
  `ExtractPackagesFromJSON` flattens package lists, and `RunSBOMPackage` carries
  only package and version. Existing rows cannot recover module boundaries.
- `persistJobSBOMPackages` lowercases package names and trims versions.
  The lookup must use that stored identity without introducing another rule.
- `BuildGateMetadata.Detected` records the execution JVM in
  `jobs.meta.gate.detected_stack`. `gateExecutionMetadata` supplies the resolved
  execution stack, including its release.
- `persistGateOutcomes` publishes SBOM rows before terminal job completion,
  including failed gates. Stored rows alone do not establish gate success.
- `ListRunSBOMRowsByJobType` aggregates current-attempt rows by run and job type.
  It cannot preserve individual job observations for this lookup.
- Jobs have `finished_at` and `started_at`, but no `created_at`. The SBOM schema
  comment currently references nonexistent `jobs.created_at`.

## Target Contract

### Request and observations

Use the shared JVM path, query validation, selectors, projection, ordering,
and JSON response from the prerequisite DD. `for` remains required.

A qualifying observation is one job satisfying all these conditions:

- `job_type` is `pre_gate` or `post_gate`.
- `status` is `Success`.
- `finished_at` is at or after the cutoff nine calendar months before query
  start, calculated in UTC.
- `meta.gate.detected_stack.language` equals `java`.
- `meta.gate.detected_stack.release` equals the canonical requested JVM string.

Search all retained attempts. Overall run status does not restrict successful
gate observations. Do not infer JVM values from images or migration targets.
Missing JVM metadata cannot qualify a job.

Match every requested coordinate and selector within the same `job_id`.
Keep pre-gate and post-gate observations separate. Multiple versions within
one job remain together in its returned observation object.

Match request coordinates exactly against stored `lib`. Ingestion already
lowercases package names; the lookup adds no further normalization. In
particular, uppercase request coordinates do not match lowercase stored names.

### HTTP behavior

Require the control-plane role, which includes access through cli-admin.
Use Ploy's existing PostgreSQL connection pool. The endpoint works without
Trino credentials.

Use a 60-second lookup timeout and an explicit 65-second write deadline.
The route must override the shorter global write deadline. Return `500` for
database failures and `504` for lookup timeout. Cancel database work when the
request is cancelled. Return no partial results or internal database details.

Preserve the deployments route's Trino-specific cancellation allowance and
`502`/`503` behavior. Source-specific error messages must identify the selected
lookup source.

## Enforcement

Keep PostgreSQL execution in the existing store authority. Add one static sqlc
query in `internal/store/queries/sboms.sql`.

Pass a parameterized JSONB request relation containing `lib`, selector kind,
selector value, and membership in `for` or `with`. Derive it from the validated
shared request. Represent each `for` entry as `Any`; preserve the selector of
each `with` entry. Membership determines the result field independently of
selector kind.

The query:

1. Selects eligible gate jobs using status, type, completion time, and JVM metadata.
2. Joins their SBOM rows to requested coordinates and selector predicates.
3. Groups matched rows by `job_id`.
4. Requires the distinct matched-coordinate count to equal the request relation's row count.
5. Aggregates `lib@ver` values separately for `for` and `with` membership,
   including exact constraints in the latter.

Parser-enforced coordinate uniqueness makes the count sufficient. Apply exact
or literal prefix version predicates before grouping. Return both text arrays
per qualifying job as one shared `Observation`, with `Found` from `for` and
`With` from `with`. Apply the prerequisite DD's normalization and response
contract, including an empty `with` array when the request omits constraints.

Subtract nine calendar months from the query-start timestamp in UTC, then
compare the resulting instant with `finished_at`. The calculation must not
depend on the PostgreSQL session timezone.

Add a thin SBOM lookup adapter in the handlers package. It converts the shared
request to store parameters and implements `JVMVersionsLookup`. The shared
handler owns parsing, validation, serialization, and result normalization.
Route wiring supplies the source-specific write deadline and error behavior.

Do not introduce a SQL dialect abstraction, a second connection pool, or
server-package dependencies in the store. Do not fetch all SBOM rows into Go
to perform matching.

## Implementation Slice

Deliver the store query and HTTP route together:

- Add the query and regenerate sqlc bindings.
- Add the lookup adapter and register the control-plane route.
- Reuse the parser, request types, response schema, and normalization from DD 1.
- Separate source-specific HTTP policy while preserving deployments behavior.
- Document gate-job observations and add PostgreSQL integration coverage.
- Correct the SBOM time-attribution comment and its corresponding test message
  to reference gate completion time.

Existing rows with qualifying JVM metadata become immediately searchable.
No node or producer rollout is required. Retention remains owned by the
existing lifecycle; the endpoint searches only retained rows.

## Completion

Verify these behaviors with PostgreSQL integration and shared HTTP tests:

- Equivalent observations produce equivalent results through both backends.
- Different jobs cannot jointly satisfy a request, including pre/post jobs
  from the same run.
- Successful pre/post gates and retained older attempts qualify, including
  successful gates belonging to runs that later failed.
- Failed or running jobs do not qualify despite uploaded SBOM rows.
- Another job's JVM metadata cannot qualify an observation.
- Missing JVM metadata and observations outside the cutoff yield no match.
- The inclusive UTC cutoff remains correct under different session timezones.
- `found` contains requested versions; `with` contains matching exact, masked,
  and unversioned constraints. Both arrays preserve the same job observation.
- Identical `found` arrays with different `with` arrays remain separate.
  Empty arrays and complete-object deduplication follow the prerequisite DD.
- Stored coordinate identity, exact versions, and mask boundaries follow DD 1.
- The route works without Trino configuration and enforces authentication,
  cancellation, timeout status, and its explicit write deadline.

Run PostgreSQL integration tests, shared handler tests, API checks, build, and
vet. Measure representative exact, masked, and unversioned requests with
`EXPLAIN (ANALYZE, BUFFERS)` and HTTP timing. Retain the query and parameters.

Report PostgreSQL buffer hits, reads, temporary I/O, and result size separately
from Trino physical-read metrics. Use the measurements to identify any later
index work; do not add speculative indexes in this slice.

## References

- [SBOM persistence and reads](../internal/store/queries/sboms.sql).
- [SBOM and job schema](../internal/store/schema.sql).
- [SBOM ingestion](../internal/server/handlers/jobs_sbom.go).
- [Gate JVM metadata](../internal/workflow/contracts/build_gate_metadata.go).
- [Execution metadata producer](../internal/workflow/step/gate_docker_metadata.go).
- [Gate outcome publication](../internal/nodeagent/gate_outcomes.go).
