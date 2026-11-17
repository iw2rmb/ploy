# Java dependency versions from Tech Radar

## Summary

Migration agents lack a shared lookup for dependency versions observed with a target Java dependency baseline.
Add one Ploy endpoint backed by Trino.
Return observed dependency combinations for the migration's JDK major version.

## Scope

- Implement `GET /v1/deployments/java/versions` in the Ploy server.
- Include Trino configuration, query execution, response types, and API documentation.
- Keep the requested route name. Its contract covers SBOM observations.
- Defer agent credential delivery, workflow wiring, and new dependency-validation steps.
- Exclude deployment-success claims, compatibility scoring, automatic version edits, caches, and PostgreSQL schema changes.

This document owns the endpoint contract.
The [Tech Radar research](../research/techradar.md) remains query background.
The endpoint replaces that note's proposed standalone `ploy-java-tools` command.

## Observed Constraint

- `handlers.RegisterRoutes` and `routeDeps` own Ploy routes and injected services.
  No Trino service or dependency-version route exists.
- `config.LoadFromEnv` owns process configuration.
  `cmd/ployd.run` later calls `applyServerTargetEnv` for server-target values.
  Trino initialization must follow that projection.
- `config.applyDefaults` sets the HTTP write timeout to 15 seconds.
  A longer query needs a route-specific write deadline.
- The Trino driver cancels timed-out queries synchronously with a separate 30-second timeout.
  A failed cancellation can trigger a second attempt before the lookup returns.
- `sbom_components` records tenant, project, commit, module, snapshot time, and dependency coordinates.
  `metadata` records repository, commit, collection time, type, and value.
  Live schema checks confirmed these fields on 2026-11-16.
- Neither table proves deployment success.
  `sbom_collection_result.success` describes a successful build and lacks a verified module-to-deployment join.
- JDK metadata can be absent at an SBOM commit.
  `lotos-api/offers-bff` commit `ed8312f2b6fe48710f0d1a5e5ad984e24eb72b3f` has tracing `2.0.3` but no same-commit Java metadata.
- Java metadata types can disagree.
  `ms/cb-platform-gateway` commit `1fbe870707b2954e21855fffba808f56c4b2e698` reports `java_version=21` and `jvm_version_maven=17`.

## Target Contract

### Request

```http
GET /v1/deployments/java/versions?for=ru.tinkoff.top.core:top-tracing-starter&with=org.springframework.boot:spring-boot@4.0.7&jdk=21
```

| Parameter | Contract |
| --- | --- |
| `for` | Required, nonempty comma-separated list of `group:artifact` coordinates. Every coordinate must occur in a matching observation. |
| `with` | Optional comma-separated list of `group:artifact@version` constraints. Every constraint must occur in the same observation. |
| `jdk` | Required, single positive integer JDK major version. The observation must match this major. |

Package versions are opaque, exact strings. Ranges and wildcard matching are unsupported.
Clients must URL-encode query values, including literal `+` characters in versions.
Trim whitespace around list items and deduplicate identical items.
Reject empty items, malformed coordinates, repeated query parameters, and unknown parameter names with `400`.
An explicitly empty optional parameter is invalid.
Reject missing, empty, non-integer, non-positive, or comma-separated `jdk` values with `400`.
`for` and `with` may contain the same coordinate.

### Observation and filtering

Use `dl.prod_sdp_sbomkeeper_aggregator.sbom_components` rows where `platform = 'jvm'`.
An observation is one `(tenant, project, commit_sha, module_id, created_at)` group.
Require non-null tenant, project, module, and snapshot time, plus a nonempty coordinate and version.
Exclude observations with missing commit SHAs because their JDK cannot be matched.

Search all retained observations. Do not restrict the endpoint to the latest module snapshot.
Apply every `for` and `with` condition within one observation.
Keep tenants, projects, commits, modules, and snapshot times separate during matching.

For each matching observation, collect every distinct `group:artifact@version` value selected by `for`.
Preserve multiple versions of one coordinate when the observation contains them.
Sort this set lexicographically.
Do not construct combinations by joining independent package-version lists.

The source can combine dependency configurations within one SBOM observation.
`conf_category` contains comma-separated categories, such as `default,test,tools`.
The endpoint reports SBOM co-occurrence, not a resolved runtime classpath.

### JDK filter

Use only `dl.prod_sdp_tech_radar.metadata` rows with `type = 'java_version'`.
Join by tenant, `project = repository`, and exact nonempty commit SHA.
Do not join Java metadata from another commit.

Accept a value only when its trimmed text is a positive decimal integer.
Include an observation only when all matching rows report the requested major.
Exclude observations when metadata rows are missing, invalid, or disagree.
Do not infer JDK values from `jvm_version_*` fields.

The filter uses the reported repository JDK major for that commit.
It does not establish the runtime JDK of a deployment.

### Response

Return `200` with a JSON object:

```json
{
  "versions": [
    ["example:client@1.2.0", "example:starter@2.0.1"],
    ["example:client@1.3.0", "example:starter@2.0.2"]
  ]
}
```

This example assumes `for=example:client,example:starter&jdk=21`.
Each inner array is a complete observed set for the requested coordinates.
Deduplicate identical sets and sort them lexicographically.
Return `{"versions": []}` when no observation matches.
Return every distinct matching set. Never silently truncate or return partial query results.
Ordering expresses neither popularity nor compatibility.

### Server integration

Use `github.com/trinodb/trino-go-client/trino` at `v1.5.0` through `database/sql`.
Keep one database handle for the server process and close it during shutdown.
Connect to `https://trino.dlh.t-infra.ru` with HTTPS Basic authentication.
Read `PLOY_TRINO_USER` and `PLOY_TRINO_PASSWORD` after `applyServerTargetEnv`.
Derive the Trino user and Basic authentication user from `PLOY_TRINO_USER`.
Construct credentials with `url.UserPassword` and the driver's configuration.
Keep credentials out of logs, responses, and agent environments.

Add the environment reader to the existing `config` package.
Keep the query and result assembly in one `internal/server/techradar` package.
Inject its lookup into the handler through the existing route-registration path.
Reuse `writeJSON`, `writeHTTPError`, and existing `control-plane` authentication.
The existing `cli-admin` role already includes `control-plane` access.

Bind request values through `QueryContext` arguments. Keep table and column names fixed.
Use the request context with a 60-second lookup timeout.
Budget for up to two driver cancellation attempts after the lookup deadline, each bounded by `trino.DefaultCancelQueryTimeout`.
Derive the route's write deadline as the lookup deadline plus twice that timeout, plus five seconds for writing.
With the driver's 30-second default, the response deadline is 125 seconds after lookup starts.
Leave the global HTTP timeout unchanged.
Cancel the Trino query when the request ends or the lookup times out.
Close result rows and check iteration errors before returning success.
Return `504` when the lookup deadline expires, even if cancellation also fails.

| Failure | HTTP status |
| --- | --- |
| Invalid request | `400` |
| Missing either Trino credential | `503` |
| Trino authentication, permission, transport, or query failure | `502` |
| Lookup timeout | `504` |

Use existing plain-text error responses. Do not expose credentials or raw upstream errors.
A lookup failure must remain distinct from `200 {"versions": []}`.
Missing Trino configuration must not prevent unrelated server routes from starting.

### Consumer rule

Use the endpoint when an unknown dependency or a version choice needs observed usage evidence.
Set `with` to the migration's fixed dependency baseline. Set `jdk` to its JDK major version.
Treat returned versions as candidates for the application's own validation.

A caller can build dependency graphs before application compilation.
On a version-resolution failure, the caller can investigate candidates from this endpoint.
Repository access failures and source compilation failures require their own diagnosis.
The caller must preserve the migration baseline and rerun the failing check after a version change.
Successful resolution and observed usage do not prove runtime compatibility.

## Enforcement

- The handler parses one query value for each input fact and rejects invalid requests before querying Trino.
- The query owns observation matching and exact-commit JDK association.
- Result assembly preserves observed sets and emits the documented JSON type in `internal/domain/api`.
- `docs/api/OpenAPI.yaml` and its path document define the public request and response schemas.
- `TestRegisterRoutesMatchesOpenAPI` enforces route-document consistency.
- Query tests distinguish co-observed sets from combinations that never occurred together.

## Implementation Slice

Implement one complete slice in the Ploy server worktree:

1. Add the Go driver and initialize the Trino handle after server-target environment projection.
2. Implement input parsing, the observation query, and deterministic response assembly.
3. Register the authenticated route with its timeout and error handling.
4. Add API types, OpenAPI documentation, configuration documentation, and focused tests.
5. Verify the endpoint against Trino with the deployed service-account credentials.

Use direct implementation. No intermediate stubs or alternative lookup path are required.
Agent access and workflow integration remain deferred until their credential-delivery contract is selected.
Reuse existing Gradle dependency checks when that integration is implemented.

## Completion

- Valid requests return only complete observed sets that satisfy every dependency constraint.
- Results include retained historical observations and deduplicate repeated sets.
- Tests cover cross-module isolation, multiple versions per coordinate, and exact version matching.
- JDK tests cover the required single value, exact-commit matching, and exclusion of missing, invalid, or conflicting metadata.
- Invalid input, unavailable Trino, timeout, and no matches remain distinguishable.
- Client cancellation cancels upstream work. The route can return after the global 15-second write timeout.
- A driver-backed test stalls cancellation requests after a lookup timeout, including a second cancellation attempt.
  The client must receive a complete `504` before the route's write deadline, including when timeout occurs during row iteration.
- Focused tests and required repository checks pass.
- A live service-account request returns the documented shape without exposing credentials.
- The retired DLH-Ranger note has no remaining links or duplicated access instructions.

## References

- [Tech Radar query research](../research/techradar.md).
- Ploy: [route registration](../internal/server/handlers/register.go),
  [environment configuration](../internal/server/config/env_loader.go),
  [HTTP defaults](../internal/server/config/defaults.go),
  [server initialization](../cmd/ployd/server.go), and [OpenAPI](../docs/api/OpenAPI.yaml).
- Existing Gradle resolution owner: [check-gradle-dependencies.init.gradle](https://gitlab.tcsbank.ru/ploy/java/-/blob/master/healing/check-gradle-dependencies.init.gradle).
- [Official Trino Go driver](https://github.com/trinodb/trino-go-client), including Basic authentication and query cancellation.
- [Trino cancellation source](https://github.com/trinodb/trino-go-client/blob/92376b7bd9a04b2ffc6761701fd01f2e677b65dd/trino/trino.go):
  `driverRows.fetch`, `Close`, `cancelAbandonedQuery`, and `cancelQueryOnServer`.
