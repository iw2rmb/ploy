# Named Specs

## Summary

Ploy specs should be reusable by name instead of only by local file path or mig project. A Git repository can publish named spec files from committed HEAD with `ploy spec push`, operators can inspect the latest published named specs with `ploy spec ls`, and `ploy run` can submit a run from a stored spec by name.

This design also updates the root mig spec contract: root `kind` is removed, root `name` and `description` are accepted, and named spec metadata is persisted with source repository identity and commit SHA.

## Scope

In scope:

- Update the mig spec schema and typed contract to remove root `kind`.
- Add optional root spec fields `name` and `description`.
- Add control-plane storage, APIs, and sqlc queries for named specs.
- Add `ploy spec push [<git-folder>]`.
- Add `ploy spec ls`.
- Extend `ploy run` to accept stored spec selectors:
  - `<spec-name>`
  - `<namespace/repo>:<spec-name>`
  - `<domain>/<namespace/repo>:<spec-name>`
- Preserve existing local-file run behavior for path selectors.

Out of scope:

- Editing specs in the control plane.
- Deleting or archiving named specs.
- Running a named spec step selector separately from the stored spec.
- Repository discovery from non-Git directories.
- Support for `.yml` discovery in `spec push`; the requested discovery suffix is `.yaml`.

## Why This Is Needed

Current single-repo runs require the caller to provide a local spec path. The server stores a spec row for each submitted run, but that row is anonymous and cannot be selected later by a stable human name. Mig projects have a separate spec-setting path, but that path is tied to a mig project rather than to a source repository and commit.

Named specs make a committed repository the publishing boundary. A published row records what spec content was prepared, where it came from, and which Git commit produced it. Later `ploy run` calls can reuse that stored prepared spec without requiring the original spec file or local bundle inputs to exist on the operator machine.

## Goals

- A published named spec is immutable and append-only.
- Published named specs are attributable to one source repository and one exact commit SHA.
- `spec push` stores the same prepared spec JSON that file-based `ploy run` would submit.
- Re-running `spec push` for the same named spec at the same source commit is idempotent.
- `spec ls` shows one latest row per `name + source`.
- `ploy run` fails clearly when a name selector is ambiguous.
- The new root spec contract is strict through the JSON schema, not ad hoc legacy checks.

## Non-goals

- No backward compatibility for root `kind`.
- No legacy-shape rejection guard for `kind`; removing it from the strict schema is the contract.
- No fallback search for unnamed specs.
- No content diff display between published spec versions.
- No implicit push during `ploy run`.

## Current Baseline Observed

- The global `ploy spec` command only exposes `schema` and `validate` in [internal/cli/app/commands_config.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/app/commands_config.go) and [internal/cli/spec/command.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/spec/command.go).
- The root mig schema currently accepts `apiVersion`, `kind`, `job_id`, `steps`, `envs`, `build_gate`, and `bundle_map` with `additionalProperties: false` in [internal/workflow/contracts/schemas/mig.schema.json](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/schemas/mig.schema.json).
- The typed mig spec still has `Kind` and no root `Name` or `Description` fields in [internal/workflow/contracts/migs_spec.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/migs_spec.go).
- The CLI prepares file specs through `specpayload.BuildSelected`, which expands refs, applies local config overlays, validates local file records, uploads spec bundles, populates `bundle_map`, marshals JSON, and validates the canonical contract in [internal/cli/specpayload/mig_run_spec.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/specpayload/mig_run_spec.go).
- Single-repo `ploy run` currently resolves a local or remote repo, builds the spec payload from a file path, and posts `{repo_url, ref, commit_sha, spec}` to `POST /v1/runs` in [internal/cli/run/run_submit.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/run/run_submit.go).
- The server `POST /v1/runs` handler always creates a new spec row from the submitted JSON and then creates a generated mig, mig repo, wave, and run in [internal/server/handlers/runs_submit.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/runs_submit.go).
- The `specs` table is append-only but only stores `id`, `name`, `spec`, `created_by`, `created_at`, and `archived_at` in [internal/store/schema.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/schema.sql). Existing `CreateSpec`, `GetSpec`, and `ListSpecs` queries do not model source repository identity, commit SHA, or commit date in [internal/store/queries/specs.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/queries/specs.sql).

## Target Contract

### Mig Spec Schema

Root accepted fields are:

- `apiVersion` optional string. Named spec discovery only considers files where this equals `ploy.mig/v1alpha1`.
- `name` optional string. When present, it must match `^[0-9a-z._-]+$`.
- `description` optional string.
- `job_id` optional string, server-injected at claim time.
- `steps` required non-empty array.
- `envs` optional string map.
- `build_gate` optional build gate config.
- `bundle_map` optional string map populated by the CLI compiler.

Root `kind` is removed from the schema and from `contracts.MigSpec`. Because the schema is strict, a spec containing root `kind` fails validation as an unknown property.

The `name` alphabet is lowercase digits, lowercase ASCII letters, dot, underscore, and dash. Spaces, slashes, colons, uppercase letters, and empty strings are invalid when `name` is present.

### Source Identity

Named specs store source identity as:

```json
{
  "domain": "github.com",
  "repo": "namespace/repo"
}
```

`domain` is the first path segment of `NormalizeRepoURLSchemless(origin_url)`.

`repo` is the remaining path after `domain/`, preserving nested namespaces. For `https://github.com/acme/service`, source is `domain=github.com`, `repo=acme/service`. Local `file://` origins cannot publish named specs unless their normalized schemeless form has at least `domain/namespace/repo` shape.

### Spec Table

The existing `specs` table remains append-only and becomes the single storage table for both anonymous run-submitted specs and named specs.

Add columns:

- `description TEXT NOT NULL DEFAULT ''`
- `source JSONB NOT NULL DEFAULT '{}'::jsonb`
- `sha TEXT NOT NULL DEFAULT ''`
- `source_committed_at TIMESTAMPTZ NULL`

Column rules:

- Anonymous run-submitted specs keep `name=''`, `description=''`, `source={}`, `sha=''`, and `source_committed_at=NULL`.
- Named specs require non-empty `name`, `source.domain`, `source.repo`, `sha`, and `source_committed_at`.
- `sha` must be a lowercase 40-hex Git commit SHA.
- `source.domain` and `source.repo` are stored without credentials and without trailing `.git`.
- `source.repo` does not include the domain.

Add an idempotency index for named specs:

```sql
CREATE UNIQUE INDEX specs_named_source_sha_name_idx
ON specs (name, (source->>'domain'), (source->>'repo'), sha)
WHERE name <> '' AND sha <> '';
```

The push loop checks for rows at the same `source + sha`; the server enforces idempotency per `name + source + sha`. This prevents a repository commit that contains multiple named specs from suppressing all but the first spec.

### Control-Plane API

Add global spec endpoints under `/v1/specs` with `auth.RoleControlPlane`.

`POST /v1/specs` publishes one named spec.

Request:

```json
{
  "name": "upgrade-java",
  "description": "Upgrade Java service dependencies",
  "source": { "domain": "github.com", "repo": "acme/service" },
  "sha": "0123456789abcdef0123456789abcdef01234567",
  "source_committed_at": "2026-06-18T10:20:30Z",
  "spec": {
    "apiVersion": "ploy.mig/v1alpha1",
    "name": "upgrade-java",
    "steps": [
      { "image": "example/image:latest" }
    ]
  },
  "created_by": "operator"
}
```

Responses:

- `201 Created` with the created spec summary when inserted.
- `200 OK` with the existing spec summary and `skipped=true` when the same `name + source + sha` already exists.
- `400 Bad Request` for invalid name, source, sha, date, or spec.
- `409 Conflict` only for a uniqueness conflict that is not the same row returned by idempotent lookup.

Summary shape:

```json
{
  "id": "specAbCd",
  "name": "upgrade-java",
  "description": "Upgrade Java service dependencies",
  "source": { "domain": "github.com", "repo": "acme/service" },
  "sha": "0123456789abcdef0123456789abcdef01234567",
  "source_committed_at": "2026-06-18T10:20:30Z",
  "created_at": "2026-06-18T10:21:00Z",
  "skipped": false
}
```

`GET /v1/specs?named=true` lists named specs. The default response is one latest row per `name + source`, ordered by `source_committed_at DESC NULLS LAST, created_at DESC, id DESC`.

`GET /v1/specs/resolve?selector=<selector>` resolves a run selector to exactly one named spec and returns the full summary plus `spec`. Resolution rules are server-side so all clients share ambiguity behavior.

Resolution selector grammar:

- `<spec-name>` matches by `name` only.
- `<namespace/repo>:<spec-name>` matches by `source.repo` and `name`, any domain.
- `<domain>/<namespace/repo>:<spec-name>` matches by `source.domain`, `source.repo`, and `name`.

Resolution result rules:

- If no row matches, return `404`.
- If more than one latest row matches, return `409` with a message listing the matching `domain/repo:name` choices.
- If exactly one latest row matches, return `200`.

### CLI: `ploy spec push`

Usage:

```text
ploy spec push
ploy spec push <git-folder>
```

Behavior:

1. Resolve the target folder to a Git worktree root with `git rev-parse --show-toplevel`.
2. Refuse to run unless the worktree is clean. Use `git status --porcelain=v1 --untracked-files=all`; any output is an error.
3. Resolve committed HEAD with `git rev-parse --verify HEAD^{commit}`.
4. Resolve the source commit date with `git show -s --format=%cI HEAD`.
5. Resolve source identity from `git remote get-url origin`, normalized through the source identity rules.
6. Find every tracked or untracked file below the worktree whose filename ends with `.yaml`, excluding `.git/`.
7. Parse each candidate only far enough to check root `apiVersion == "ploy.mig/v1alpha1"` and non-empty root `name`.
8. For each matching file, build the prepared spec with the same path used by file-based `ploy run`: `specpayload.BuildSelected(ctx, base, client, path, "", nil, "", false, "")`.
9. Publish each prepared spec with `POST /v1/specs`.
10. Print a table with every inserted or skipped spec.

`spec push` does not accept CLI overrides for envs, image, command, or step selection. Published specs must be exactly what the committed file defines plus the normal local config overlay and bundle compilation rules already used by `ploy run`.

Output columns mirror `spec ls` and add a state column:

```text
STATE    NAME          SOURCE                  SHA       DATE
updated  upgrade-java  github.com/acme/svc     01234567  2026-06-18T10:20:30Z
skipped  heal-gradle   github.com/acme/svc     01234567  2026-06-18T10:20:30Z
```

If no matching named specs are found, exit successfully and print `No named specs found`.

### CLI: `ploy spec ls`

Usage:

```text
ploy spec ls
```

Behavior:

- Calls `GET /v1/specs?named=true`.
- Prints one row per latest `name + source` combination.
- Columns: `NAME`, `SOURCE`, `SHA`, `DATE`.
- `SOURCE` is `domain/namespace/repo`.
- `SHA` is the first eight characters of `sha`.
- `DATE` is `source_committed_at` in RFC3339.

### CLI: `ploy run` Named Selector

Current local-file behavior stays first:

- Existing paths, absolute paths, `./`, `../`, and existing directories continue to resolve as local specs.
- Existing `<spec-path>:<step-name>` works only when the left side resolves to a local spec path.

If the first positional arg does not resolve as a local spec path, `ploy run` treats it as a named spec selector.

Usage examples:

```text
ploy run upgrade-java ./repo
ploy run acme/service:upgrade-java ./repo
ploy run github.com/acme/service:upgrade-java ./repo
ploy run upgrade-java acme/service:main
```

Named run behavior:

1. Resolve the named spec through `GET /v1/specs/resolve?selector=...`.
2. Use the returned stored prepared `spec` bytes as the run payload.
3. Resolve the second argument as the target repository path or repository selector using the current `resolveSourceRepo` flow.
4. Submit through the existing `POST /v1/runs` path.

The server still creates a new anonymous run spec row for the submitted run unless `POST /v1/runs` is extended to accept `spec_id`. A follow-up optimization may pass `spec_id` to avoid duplicate rows, but named-spec correctness does not depend on that change.

Failure behavior:

- Unknown selector: `run submit: named spec not found: <selector>`.
- Ambiguous selector: `run submit: named spec selector <selector> is ambiguous: github.com/acme/service:upgrade-java, gitlab.example.com/acme/service:upgrade-java`.
- Invalid selector grammar: `run submit: invalid named spec selector: <selector>`.

## Implementation Notes

### Schema and Contract Types

- Update [internal/workflow/contracts/schemas/mig.schema.json](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/schemas/mig.schema.json): remove `kind`, add `name` with regex, add `description`.
- Update [internal/workflow/contracts/migs_spec.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/migs_spec.go): remove `Kind`, add `Name` and `Description`.
- Update fixtures and examples such as [docs/schemas/mig.example.yaml](/Users/v.v.kovalev/@iw2rmb/ploy/docs/schemas/mig.example.yaml) to remove `kind`.
- Keep strictness in the schema. Do not add manual `kind` rejection code or legacy tests.

### Store and Migrations

- Update [internal/store/schema.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/schema.sql) and the active Tern migration chain with a new migration after version `2`.
- Increment `TargetSchemaVersion` in [internal/store/migrations.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/migrations.go).
- Extend [internal/store/queries/specs.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/queries/specs.sql) with:
  - `CreateNamedSpec`
  - `GetNamedSpecByNameSourceSHA`
  - `ListLatestNamedSpecs`
  - `ResolveLatestNamedSpecByName`
  - `ResolveLatestNamedSpecByRepoName`
  - `ResolveLatestNamedSpecByDomainRepoName`
- Regenerate sqlc output.
- Store `source` as JSONB to match the requested `source` object while indexing its stable keys.

### Server Handlers

- Add `registerGlobalSpecRoutes` or extend `registerSpecBundleRoutes` only if the route ownership remains clear. Global named specs are not spec bundles, so a separate registration function is preferred.
- Add DTOs under `internal/domain/api` for named spec source, publish request, summary, list response, and resolve response.
- Validate the named publish request before inserting:
  - root request `name` must equal the prepared spec root `name`.
  - request `description` must equal root `description` when the root field exists; otherwise use the request field as the stored summary description.
  - `spec` must pass `contracts.ParseMigSpecJSON`.
  - `spec.apiVersion` must equal `ploy.mig/v1alpha1` for named publishes.
- Keep `/v1/migs/{mig_ref}/specs` unchanged except for the extended `CreateSpec` parameter shape.
- Keep `POST /v1/runs` unchanged for initial implementation. It can still receive stored prepared spec bytes from the CLI.

### CLI Spec Commands

- Move `ploy spec` from a simple `Handle` router toward command structs if needed, matching `internal/cli/run` style for HTTP commands.
- Reuse `common.ResolveControlPlaneHTTP` for `spec push` and `spec ls`.
- Add Git helpers near the spec command, not in run submission, because Git cleanliness and source metadata are publishing concerns.
- Discovery should parse YAML with `gopkg.in/yaml.v3` into a small root probe struct before building the full prepared payload. Files without matching `apiVersion` and `name` are ignored, not reported as invalid specs.
- If a file has matching `apiVersion` and `name` but full preparation fails, `spec push` fails the command and reports the file path.

### CLI Run Resolution

- Extend `SubmitOptions` naming from `SpecPath` to `SpecSelector` or keep the field but avoid path-only assumptions in new helpers.
- Keep `splitRunSpecSelector` for local paths only. Named spec selectors use the colon to separate source and spec name, so a named selector must not pass through step-selection parsing.
- Add a helper such as `resolveRunSpecPayload`:
  - if the selector resolves as a local file or directory, build through `buildRunSubmitSpecPayload`;
  - otherwise resolve and fetch the named spec payload from the control plane.
- Preserve the existing two-argument run form where the second arg is the target repo path or repo selector.

### OpenAPI and Docs

- Add `/v1/specs` and `/v1/specs/resolve` to [docs/api/OpenAPI.yaml](/Users/v.v.kovalev/@iw2rmb/ploy/docs/api/OpenAPI.yaml).
- Add path files under `docs/api/paths/` and schema definitions under `docs/api/components/schemas/controlplane.yaml`.
- Update [cmd/ploy/README.md](/Users/v.v.kovalev/@iw2rmb/ploy/cmd/ploy/README.md) for `ploy spec push`, `ploy spec ls`, and named `ploy run` selectors.

## Milestones

### Milestone 1: Root Spec Contract

Scope:

- Remove root `kind`.
- Add root `name` and `description`.
- Update examples and validation tests.

Expected results:

- Specs with `kind` fail schema validation because the schema is strict.
- Specs with valid `name` and `description` validate.

Testable outcome:

- `go test ./internal/workflow/contracts ./internal/cli/specpayload`
- `go run ./cmd/ploy spec validate docs/schemas/mig.example.yaml`

### Milestone 2: Named Spec Storage and API

Scope:

- Add DB columns, migration, sqlc queries, API DTOs, server handlers, and OpenAPI docs.

Expected results:

- Named specs can be published idempotently.
- Latest named specs can be listed.
- Name selectors resolve exactly or fail with 404/409.

Testable outcome:

- Store tests cover insertion, idempotency, and latest-per-combo ordering.
- Handler tests cover publish, list, resolve none, resolve ambiguous, and resolve exact.
- `go test ./internal/store ./internal/server/handlers ./docs/api`

### Milestone 3: CLI Publish and List

Scope:

- Add `ploy spec push [<git-folder>]` and `ploy spec ls`.

Expected results:

- `spec push` refuses dirty worktrees.
- `spec push` discovers only `.yaml` files with matching `apiVersion` and `name`.
- `spec push` stores prepared specs with bundle_map populated as `ploy run` would.
- `spec ls` renders latest named specs.

Testable outcome:

- CLI tests with a temp Git repo and `httptest.Server` cover clean, dirty, skipped, updated, and no-match cases.
- `go test ./internal/cli/spec ./internal/cli/app`

### Milestone 4: Named `ploy run`

Scope:

- Extend run submission to load a named stored spec when the first arg is not a local path.

Expected results:

- Existing path-based runs still work.
- Named selectors submit stored prepared spec bytes.
- Ambiguous named selectors return a clear error before run submission.

Testable outcome:

- CLI run tests cover local path precedence, name-only exact, repo/name exact, domain/repo/name exact, ambiguous, unknown, and remote target repo cases.
- `go test ./internal/cli/run ./internal/cli/app`

## Acceptance Criteria

- `kind` is removed from the root spec schema, typed contract, docs, tests, and examples.
- Root `name` validates with `^[0-9a-z._-]+$`; invalid names are rejected by schema validation.
- `ploy spec push` from a clean Git repository publishes every `.yaml` file with `apiVersion: ploy.mig/v1alpha1` and a valid root `name`.
- `ploy spec push` refuses dirty worktrees, including untracked files.
- Publishing the same `name + source + sha` twice reports `skipped` and does not insert a duplicate row.
- `ploy spec ls` shows latest named specs with `name`, `domain/namespace/repo`, short SHA, and commit date.
- `ploy run <name> <repo>` uses the stored prepared spec when exactly one latest named spec matches.
- `ploy run <namespace/repo>:<name> <repo>` and `ploy run <domain>/<namespace/repo>:<name> <repo>` narrow resolution as specified.
- Ambiguous named run selectors fail before `POST /v1/runs`.
- Existing local file run forms continue to work, including `<spec-path>:<step-name>`.
- OpenAPI verification passes.

## Risks

- Selector ambiguity can surprise users when the same spec name is published by multiple repositories. The explicit 409 choices are required to make correction obvious.
- `source` extraction from arbitrary Git remotes can be lossy for non-hosted or file remotes. The publish command should fail when it cannot derive `domain` and `repo` instead of storing weak identity.
- Prepared specs contain `bundle_map` IDs for uploaded spec bundles. `spec push` must use the same bundle upload code as `ploy run`; otherwise stored specs can be syntactically valid but not runnable.
- Local config overlays make published output depend on the operator environment. This is already true for `ploy run`; the DD preserves that behavior rather than creating a second preparation mode.
- Keeping `POST /v1/runs` unchanged duplicates the stored named spec into a new anonymous spec row per run. This is acceptable for the first implementation but may need cleanup if spec-row volume becomes noisy.

## References

- [internal/workflow/contracts/schemas/mig.schema.json](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/schemas/mig.schema.json)
- [internal/workflow/contracts/migs_spec.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/contracts/migs_spec.go)
- [internal/cli/specpayload/mig_run_spec.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/specpayload/mig_run_spec.go)
- [internal/cli/run/run_submit.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/cli/run/run_submit.go)
- [internal/server/handlers/runs_submit.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/runs_submit.go)
- [internal/store/schema.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/schema.sql)
- [internal/store/queries/specs.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/queries/specs.sql)
