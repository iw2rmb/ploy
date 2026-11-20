# Flexible JVM dependency constraints

## Summary

The deployments endpoint requires exact versions in `with`. Add masked and
unversioned selectors. Return each observation with separate `found` and `with`
arrays for requested versions and matched constraint versions.

## Scope

Extend `GET /v1/deployments/jvm/{jvm}/versions` and establish the shared request
contract for [Ploy SBOM version lookup](sbom-jvm-versions.md).

Keep `for` required and `with` optional. Preserve the existing JVM matching,
nine-month window, and `(commit_sha, created_at, module_id)` observation boundary.
Replace the response's inner arrays with observation objects for both endpoints.
Existing exact-version requests retain their matching observations; their
response shape changes, and their matched constraints appear in `with`.
Standalone lowercase `x` components become reserved mask syntax.

General version ranges, automatic relaxation, source federation, caching, and
changes to SBOM ingestion remain outside this DD.

## Observed Constraint

- `parseJVMVersionsRequest` requires `group:artifact@version` for every `with`
  entry. It already owns coordinate validation, deduplication, and conflicts.
- `deployments.Constraint` stores one exact version. `buildQuery` uses equality
  in source filtering and observation qualification, and projects only `for`.
- `deployments.lookup` owns result sorting and deduplication. Both lookup
  endpoints need this behavior, while their storage and observation keys differ.
- `JVMVersionsLookup` depends on `deployments.Request`. Shared request types must
  have an owner independent of either storage source.
- `JVMVersionsResponse.Versions` contains `[][]string`. It cannot distinguish
  requested versions from the versions that satisfied constraints.

## Target Contract

### Selectors

Accept these `with` entries:

| Entry | Match | Returned in `with` |
| --- | --- | --- |
| `g:a@4.0.7` | Exact version | Yes |
| `g:a@4.x.x` | Literal prefix `4.` | Yes |
| `g:a@4.0.x` | Literal prefix `4.0.` | Yes |
| `g:a` | Coordinate, regardless of version | Yes |

Masks use literal dotted prefixes. They include qualifiers such as
`4.0.7-RC1` and `4.0.7-SNAPSHOT`. They do not parse Maven or semantic versions.
Consequently, `4.x.x` also matches `4.0` and `4.foo`.

`M` and `N` contain one or more ASCII decimal digits. Preserve their spelling.
Reserve standalone lowercase `x` dot-components for `M.x.x` and `M.N.x`.
Reject other forms containing those components, such as `4.x` and `4.x.7`.
Other version strings retain opaque exact matching; wildcard and range
characters have no special meaning. Reject an empty version after `@`.

The parser retains existing list trimming and query-parameter validation.
Coordinates remain unique and disjoint across `for` and `with`. Identical entries
deduplicate. Different selectors for the same coordinate return `400` before
lookup, including an unversioned entry combined with a masked or exact entry.

### Observations and results

Every `for` coordinate and `with` selector must match within one observation.
Each object in `versions` has two required arrays of `group:artifact@version`
strings:

- `found` contains every observed version of each `for` coordinate.
- `with` contains every matching version of each `with` coordinate, including
  exact, masked, and unversioned selectors.

Return actual observed versions in both arrays. An unversioned selector admits
every observed version of its coordinate. A masked selector contributes only
versions matching its prefix. An exact selector contributes its matching version.

Multiple matching versions remain together within their observation. Do not
generate combinations across observations. When the request omits `with`,
return `"with": []`. Neither field may be omitted or serialized as null.

Sort and deduplicate each array lexicographically. Deduplicate complete objects
using both arrays. Identical `found` arrays with different `with` arrays remain
separate. Sort objects lexicographically by `found`, then by `with`.

Return complete results without truncation. No matches return
`{"versions":[]}`. Preserve the empty suffix used for null versions.

For example, `for=g:library&with=g:baseline@4.0.x` can return:

```json
{
  "versions": [
    {
      "found": ["g:library@2.0.1"],
      "with": ["g:baseline@4.0.6"]
    },
    {
      "found": ["g:library@2.0.1"],
      "with": ["g:baseline@4.0.7"]
    }
  ]
}
```

Each object belongs to one matching observation. These objects remain distinct
because their matched baseline versions differ.

## Enforcement

Move request types into `internal/domain/jvmversions`:

- `Coordinate` contains `Group` and `Artifact`.
- `Selector` contains `Kind` (`Any`, `Exact`, or `Prefix`) and its effective
  `Value`. `Any` has an empty value; masks store only their literal prefix.
- `Constraint` contains a coordinate and its selector.
- `Request` contains `JVM`, `For`, and `With`.

Add the shared result type in the same package:

```go
type Observation struct {
    Found []string `json:"found"`
    With  []string `json:"with"`
}
```

Change `api.JVMVersionsResponse.Versions` to `[]jvmversions.Observation`.
Both endpoints use this result type and response schema.

Remove the original request types from `deployments`. Do not retain aliases or
a second raw-mask representation. Neither the domain types nor PostgreSQL
storage may depend on the deployments package.

The existing HTTP parser remains the validation authority. Keep
`JVMVersionsLookup` in handlers with this signature:

```go
Lookup(context.Context, jvmversions.Request) ([]jvmversions.Observation, error)
```

Move result normalization into one function in the shared handler path. It
normalizes both arrays, deduplicates and sorts complete objects, and converts
empty results and fields to empty arrays. Remove the original normalization
implementation from the Trino reader.

Derive result membership from the request's `For` and `With` lists. Selector
kind controls version matching, not which result field receives a coordinate.
Each backend must preserve this membership through aggregation, including for
`Any` selectors used in either list.

Keep SQL in each source's existing owner. Within `deployments.buildQuery`, reuse
selector predicate construction for source filtering and observation
qualification. Parameterize exact values and prefixes. Use literal prefix
matching rather than interpreting SQL wildcard characters from user input.

## Implementation Slice

Deliver the deployments extension and shared request boundary together:

- Move the request types and extend parsing with the three selector kinds.
- Apply coordinate-only, exact, or prefix predicates in `components`.
- Keep one qualification predicate per requested coordinate in `HAVING`.
  Apply the corresponding selector to each `with` predicate.
- Aggregate `for` rows into `found` and all matching `with` rows into `with`.
  Return both aggregates for each observation and decode them into `Observation`.
- Move result normalization into the HTTP path and update its callers and tests.
- Replace the response type and schema, endpoint examples, and response comments
  with the object contract. Update consumers of the old inner-array shape in
  the same slice; provide no parallel legacy response format.

Preserve the Trino lookup's 60-second timeout, 125-second write deadline,
cancellation behavior, and source-specific errors. Add no second validation
layer or generic SQL builder.

The completed slice serves exact, masked, and unversioned requests through the
existing endpoint. The PostgreSQL route remains the separate delivery in DD 2.

## Completion

Verify these behaviors with parser, HTTP, and live Trino fixture tests:

- Existing exact requests retain their matching observations, including literal
  `%`, `_`, `+`, and quotes. Reserved malformed `x` masks return `400`.
- `found` contains only `for` coordinates. `with` includes matched exact,
  masked, and unversioned constraints with actual observed versions.
- Requests without `with` return an empty `with` array. Empty results return
  `{"versions":[]}`; required arrays never serialize as null.
- Identical `found` arrays with different `with` arrays remain separate.
  Identical complete objects collapse, with deterministic ordering by both arrays.
- `4.x.x` excludes `40.*`; `4.0.x` excludes `4.01.*`. Qualifiers and literal
  prefix behavior match the contract.
- Conflicting selectors and overlaps fail before database execution.
- Module and snapshot boundaries prevent cross-observation matches.
- Multiple versions, null versions, empty results, and duplicate observations
  preserve the response contract.
- Source filtering excludes versions outside a mask before projection.

Run focused domain, deployments, handler, and API tests, plus build and vet.
Compare exact, masked, and unversioned requests for results, elapsed time,
physical reads, worker CPU, network, and peak memory. Record the exact requests
and query IDs with each measurement.

## References

- [HTTP parsing and lookup](../internal/server/handlers/jvm_versions.go).
- [Trino query and reader](../internal/server/deployments/deployments.go).
- [Current endpoint contract](../docs/api/paths/deployments_jvm_versions.yaml).
- [Shared response schema](../docs/api/components/schemas/deployments.yaml).
