# Server-Owned GitLab Hydration

## Summary
Move GitLab access authority fully into `ployd`.

`ployd` reads GitLab requisites only from `PLOY_GITLAB_DOMAIN` and
`PLOY_GITLAB_TOKEN`, keeps them in memory only, resolves source commits itself,
and serves checked-out repository snapshots to worker nodes through:

```http
GET /v1/runs/{run_id}/repos/{repo_id}/snapshot
```

Worker nodes no longer receive GitLab PATs, no longer run authenticated GitLab
network fetches, and no longer create GitLab merge requests. Existing MR
creation machinery is removed until a future server-owned MR design is added.

## Scope
In scope:

- Remove `ploy config gitlab ...` CLI commands.
- Remove `GET /v1/config/gitlab` and `PUT /v1/config/gitlab`.
- Make `PLOY_GITLAB_DOMAIN` and `PLOY_GITLAB_TOKEN` the only GitLab requisite
  source for `ployd`.
- Keep GitLab requisites in server memory only.
- Remove claim-time and spec-time GitLab PAT/domain propagation.
- Add server-owned repository snapshot endpoint:
  `GET /v1/runs/{run_id}/repos/{repo_id}/snapshot`.
- Change node hydration to download and unpack the snapshot endpoint.
- Remove GitLab MR creation flow, actions, flags, spec fields, and node-side MR
  execution.

Out of scope:

- Reintroducing MR creation in any form.
- Short-lived GitLab token minting.
- Git proxying.
- Supporting per-run or per-spec GitLab credentials.
- Backward compatibility for old `gitlab_pat`, `gitlab_domain`,
  `mr_on_success`, or `mr_on_fail` contracts.

## Why This Is Needed
Current GitLab auth crosses too many boundaries.

- Server env defaults are injected into claimed specs as `gitlab_pat` and
  `gitlab_domain` in
  `internal/server/handlers/claim_spec_mutator_base.go`.
- Node hydration extracts Git auth from manifest options in
  `internal/workflow/step/hydrator.go`.
- `internal/gitauth` prepares process-scoped Git auth for node-side Git
  operations.
- MR creation keeps additional GitLab PAT paths in nodeagent and server action
  code.

This means a long-lived GitLab PAT can be present in claim payloads, node memory,
manifest options, and node-side execution paths. The target design keeps the
long-lived PAT only inside `ployd` and makes node hydration a server API
download.

## Goals
- GitLab domain and token have one source: server process environment.
- GitLab domain and token are never persisted in Ploy DB.
- GitLab token is never sent to worker nodes.
- Worker nodes hydrate repos without knowing GitLab credentials.
- Snapshot identity is repo/run scoped: `(run_id, repo_id)`.
- `ployd` remains the authority for `source_commit_sha`.
- Repository snapshots include `.git` so existing local Git diff and repo SHA
  behavior remains Git-native.
- MR machinery is fully removed, not left as inert compatibility code.

## Non-goals
- No per-run GitLab override.
- No CLI/admin mutation of GitLab config.
- No server API that returns GitLab credentials.
- No node-side GitLab clone/fetch auth.
- No GitLab MR creation.
- No compatibility shim for old GitLab spec keys.

## Current Baseline (Observed)
- `PLOY_GITLAB_DOMAIN` and `PLOY_GITLAB_TOKEN` are loaded into server config in
  `internal/server/config/env_loader.go`.
- `GitLabConfig` also supports YAML fields `domain`, `token`, and `token_file`
  in `internal/server/config/types.go`.
- `GET /v1/config/gitlab` and `PUT /v1/config/gitlab` are registered in
  `internal/server/handlers/register.go`.
- `putGitLabConfigHandler` updates `ConfigHolder` in memory only in
  `internal/server/handlers/config_gitlab.go`.
- Claim-time mutation injects `gitlab_pat` and `gitlab_domain` into claimed
  specs in `internal/server/handlers/claim_spec_mutator_base.go`.
- `source_commit_sha` is resolved by `ployd` during run/repo creation through
  `resolveSourceCommitSHAFromContext` in
  `internal/server/handlers/repo_sha_seed.go`.
- `run_repos.source_commit_sha` and `run_repos.repo_sha0` are inserted together
  by `CreateRunRepo` in `internal/store/queries/run_repos.sql`.
- Claim response sends `commit_sha` from `run_repos.source_commit_sha`, falling
  back to `job.repo_sha_in`, in
  `internal/server/handlers/nodes_claim_response.go`.
- Node hydration currently fetches Git repositories using node-side Git in
  `internal/worker/hydration/git_fetcher.go`.
- MR creation is wired through:
  - `POST /v1/runs/{run_id}/repos/{repo_id}/mr`
  - `internal/server/handlers/runs_repo_mr_actions.go`
  - `internal/server/handlers/jobs_complete_service_post_actions.go`
  - `internal/nodeagent/execution_mr.go`
  - `internal/nodeagent/gitlab`
  - CLI flags and spec fields for GitLab/MR options.

## Target Contract or Target Architecture
### GitLab Configuration
`ployd` accepts only:

```text
PLOY_GITLAB_DOMAIN
PLOY_GITLAB_TOKEN
```

Rules:

- These values are read during server process startup.
- These values are kept in memory only.
- These values are not written to DB.
- These values are not returned by any HTTP endpoint.
- These values are not injected into specs, claim payloads, manifests, job env,
  action payloads, or artifacts.
- YAML config fields for GitLab credentials are removed.
- `token_file` support is removed.
- `ploy config gitlab show|set|validate` is removed.
- `/v1/config/gitlab` is removed.

### Spec Contract
Remove these fields from the active Mig spec contract:

```yaml
gitlab_pat
gitlab_domain
mr_on_success
mr_on_fail
```

Schema/contract validation defines accepted current fields. Do not add
legacy-specific rejection code for these keys unless strict key rejection is
implemented by the schema.

### Source Commit Authority
`ployd` calculates `source_commit_sha` before creating `run_repos` rows.

Inputs:

- clean repo URL from `repos.url`
- base ref from `mig_repos.base_ref` or run submit request
- GitLab auth from in-memory server config

Algorithm:

1. Trim `repo_url` and `base_ref`.
2. Build candidate refs:
   - raw `base_ref`
   - `refs/heads/<base_ref>` when `base_ref` is not already a full ref
   - `refs/tags/<base_ref>` when `base_ref` is not already a full ref
3. Run `git ls-remote` from `ployd` for each candidate.
4. Use the first 40-hex SHA returned.
5. Insert both `source_commit_sha` and `repo_sha0` with that SHA.
6. Reject run/repo creation if the source commit cannot be resolved.

`source_commit_sha` remains immutable for a run repo.

### Snapshot Endpoint
Register one worker endpoint:

```http
GET /v1/runs/{run_id}/repos/{repo_id}/snapshot
```

Auth:

- Role: `RoleWorker`.
- Query-token auth is not allowed.
- Request must include the node identity in the same form used by worker upload
  endpoints:

```http
PLOY_NODE_UUID: <node_id>
```

Request body: none.

Success response:

```http
200 OK
Content-Type: application/gzip
X-Ploy-Repo-SHA: <source_commit_sha>
X-Ploy-Repo-URL: <clean repo_url>
X-Ploy-Repo-Base-Ref: <repo_base_ref>
```

Response body:

- gzip-compressed tar stream.
- Tar root is the repository workspace contents.
- The tar stream includes `.git`.
- The checkout is at `run_repos.source_commit_sha`.
- `.git/config` must not contain credentials.
- The remote URL stored in `.git/config` must be the clean repo URL.

The node unpacks the response directly into the workspace directory and then
continues with local Git operations.

Failure responses:

- `400` when `run_id`, `repo_id`, or `PLOY_NODE_UUID` is invalid.
- `401` when caller identity is missing.
- `403` when caller is not a worker or the node is not assigned current work for
  this run repo.
- `404` when `(run_id, repo_id)` does not identify a run repo.
- `409` when `source_commit_sha` is empty or the run repo is not snapshot-ready.
- `502` when GitLab fetch fails.
- `504` when GitLab fetch times out.

### Snapshot Authorization Query
The endpoint authorizes by current node ownership of work for `(run_id, repo_id)`.
`job_id` is not part of the endpoint path because the snapshot identity is
run/repo scoped.

Required job authorization query:

```sql
SELECT 1
FROM jobs
WHERE run_id = @run_id
  AND repo_id = @repo_id
  AND node_id = @node_id
  AND status = 'Running'
LIMIT 1;
```

Required action authorization query:

```sql
SELECT 1
FROM run_repo_actions
WHERE run_id = @run_id
  AND repo_id = @repo_id
  AND node_id = @node_id
  AND status = 'Running'
LIMIT 1;
```

The request is authorized when either query returns a row. Otherwise it returns
`403`.

### Snapshot Metadata Query
The endpoint resolves snapshot metadata from `(run_id, repo_id)`:

```sql
SELECT
  rr.run_id,
  rr.repo_id,
  rr.repo_base_ref,
  rr.repo_target_ref,
  rr.source_commit_sha,
  r.url AS repo_url
FROM run_repos rr
JOIN repos r ON r.id = rr.repo_id
WHERE rr.run_id = @run_id
  AND rr.repo_id = @repo_id;
```

Validation:

- `source_commit_sha` must be a lowercase 40-hex SHA.
- `repo_url` must be a valid accepted repo URL.
- If `PLOY_GITLAB_DOMAIN` is non-empty, authenticated GitLab access is used only
  when `repo_url` host matches that domain.
- Git auth is attached only to the server-side Git subprocess and never embedded
  in a URL or persisted clone config.

### Snapshot Materialization
`ployd` materializes snapshots with a server-side fetcher.

Required behavior:

- Cache by clean repo URL and full commit SHA.
- Existing cache layout may be reused:
  `$PLOYD_CACHE_HOME/git-clones/<domain>/<namespace>/<repo>/<full_sha>/`.
- On cache hit, verify exact `HEAD == source_commit_sha`.
- On cache miss, clone/fetch using server-side Git auth and checkout
  `source_commit_sha`.
- Sanitize `origin` after clone/fetch.
- Stream a tar.gz of a verified clean clone.
- Do not write Ploy metadata files into cached repo directories.

The streamed snapshot is a read input to the node. The node may mutate its
workspace after unpacking; those mutations are not written back to the server
snapshot cache.

### Node Hydration Contract
Node hydration for repo jobs is unconditional:

1. Claim response contains `run_id`, `repo_id`, `repo_url`, `base_ref`,
   `target_ref`, and `commit_sha` as today.
2. Node creates or clears the workspace.
3. Node calls:

   ```http
   GET /v1/runs/{run_id}/repos/{repo_id}/snapshot
   PLOY_NODE_UUID: <node_id>
   ```

4. Node unpacks the tar.gz into the workspace.
5. Node validates local `git rev-parse HEAD` equals claim `commit_sha`.
6. Node continues with local Git diff, repo SHA, baseline commit, and runtime
   execution.

Node no longer:

- parses GitLab auth from specs or manifests,
- calls GitLab to clone/fetch repo sources,
- stores GitLab PATs in memory,
- uses `internal/gitauth` for hydration.

### MR Removal Contract
Remove MR creation as a product/runtime feature.

Remove endpoints:

```http
POST /v1/runs/{run_id}/repos/{repo_id}/mr
```

Remove runtime action type and scheduling for MR creation:

- no explicit MR action creation route,
- no automatic MR action enqueue on terminal run repo status,
- no `mr_create` action execution,
- no `mr_url` propagation from action completion.

Remove node-side implementation:

- `internal/nodeagent/execution_mr.go`
- `internal/nodeagent/gitlab`
- MR push path using GitLab PAT
- MR-specific manifest options and typed options

Remove CLI/spec surfaces:

- `ploy config gitlab ...`
- `--gitlab-pat`
- `--gitlab-domain`
- `--mr-success`
- `--mr-fail`
- `gitlab_pat`
- `gitlab_domain`
- `mr_on_success`
- `mr_on_fail`

Any future MR design must be server-owned and must not reintroduce GitLab PATs
into node claims or node runtime.

## Implementation Notes
### Server Config
- Keep env loading for `PLOY_GITLAB_DOMAIN` and `PLOY_GITLAB_TOKEN`.
- Remove GitLab YAML config fields and token-file loading.
- Remove `ConfigHolder` GitLab mutation methods and state unless still needed
  as a read-only server config holder. Prefer passing typed server config to
  handlers that need GitLab access.
- Remove GitLab config API handlers and tests.

### Run Creation
- Thread server GitLab config into:
  - `createSingleRepoRunHandler`
  - `createMigRunHandler`
  - `addRunRepoHandler`
- Replace `gitAuthOptionsFromSpec(...)` with auth derived from server env config.
- Keep `source_commit_sha` resolution before durable run repo creation.

### Snapshot Handler
- Add `getRunRepoSnapshotHandler`.
- Register:

  ```go
  s.RegisterRouteFunc(
      "GET /v1/runs/{run_id}/repos/{repo_id}/snapshot",
      getRunRepoSnapshotHandler(deps.st, deps.snapshotService),
      auth.RoleWorker,
  )
  ```

- Add store queries for authorization and metadata.
- Add a snapshot service that owns server-side clone/cache/tar streaming.

### Nodeagent
- Replace repo hydration fetch path with snapshot download/unpack.
- Keep local Git requirements because `.git` is present.
- Remove GitLab auth fields from `RunOptions`, manifest options, and parsing.
- Remove MR job dispatch and MR execution paths.

### Contracts and Docs
- Remove GitLab and MR fields from Mig spec contracts.
- Remove OpenAPI entries for GitLab config and MR creation.
- Update CLI help and docs so GitLab requisites are described only as `ployd`
  environment variables.

## Milestones
### Milestone 1: Remove Mutable GitLab Configuration
Scope:

- Delete `ploy config gitlab ...`.
- Delete `/v1/config/gitlab`.
- Delete YAML `gitlab.token`, `gitlab.domain`, and `gitlab.token_file`.
- Keep env-only server GitLab config.

Expected results:

- GitLab requisites can only enter `ployd` through process env.
- No admin route returns or mutates GitLab config.

Testable outcome:

- CLI help has no `config gitlab`.
- API route registration has no `/v1/config/gitlab`.
- Server config tests cover env-only GitLab config.

### Milestone 2: Server-Owned Source SHA Auth
Scope:

- Remove GitLab auth fields from spec parsing and claim mutation.
- Resolve source SHAs using server env config.
- Remove per-run GitLab credentials from CLI/spec payload builders.

Expected results:

- `source_commit_sha` still resolves for private GitLab repos.
- Specs and claim payloads do not contain GitLab PAT/domain.

Testable outcome:

- Run creation for a private GitLab repo succeeds when `ployd` env has GitLab
  requisites.
- Captured claim response has no `gitlab_pat` or `gitlab_domain`.

### Milestone 3: Snapshot Endpoint and Node Hydration
Scope:

- Add snapshot metadata and authorization queries.
- Add snapshot handler and server-side snapshot service.
- Switch node hydration to snapshot download/unpack.

Expected results:

- Node can hydrate repo workspaces without GitLab credentials.
- Snapshot contains `.git` and exact `source_commit_sha`.

Testable outcome:

- Worker assigned to a running job gets `200`.
- Unassigned worker gets `403`.
- Unpacked workspace passes `git rev-parse HEAD == source_commit_sha`.
- Node-side GitLab clone/fetch tests are removed or rewritten for snapshot
  hydration.

### Milestone 4: Remove MR Machinery
Scope:

- Delete MR action route and scheduling.
- Delete node MR execution and GitLab MR client.
- Delete MR spec/CLI fields and status display fields.

Expected results:

- No code path creates GitLab MRs.
- No node path requires GitLab PAT for push/API operations.

Testable outcome:

- `rg "mr_on_success|mr_on_fail|gitlab_pat|gitlab_domain|mr_create|CreateMR"`
  returns no active runtime contract references.
- API route registration has no `/mr` endpoint.

## Acceptance Criteria
- `PLOY_GITLAB_DOMAIN` and `PLOY_GITLAB_TOKEN` are the only accepted GitLab
  requisite inputs.
- GitLab requisites are stored only in `ployd` memory.
- GitLab requisites are never stored in DB, claim specs, manifests, node env, or
  artifacts.
- `GET /v1/runs/{run_id}/repos/{repo_id}/snapshot` is the only node repo
  hydration API.
- Snapshot authorization is enforced by current node ownership of a running
  job/action for `(run_id, repo_id)`.
- Snapshot content includes `.git`, has sanitized origin config, and is checked
  out at `run_repos.source_commit_sha`.
- Nodes do not call GitLab for repo hydration.
- MR creation code paths, endpoints, CLI flags, spec fields, and tests are gone.

## Risks
- Server snapshot streaming moves clone bandwidth and CPU from nodes to `ployd`.
- Large repos can make snapshot responses expensive; cache correctness and
  streaming backpressure matter.
- Including `.git` is required for current local Git diff/repo SHA behavior, but
  increases payload size compared with a plain source archive.
- Startup/retry behavior must avoid partially unpacked workspaces on node.
- Authorization must reject stale or unassigned workers; otherwise snapshots
  become a repo-read escalation path.
- Removing MR fields can break callers that still submit old specs; this is
  accepted because backward compatibility is not required.

## References
- `internal/server/config/env_loader.go`
- `internal/server/config/types.go`
- `internal/server/handlers/register.go`
- `internal/server/handlers/claim_spec_mutator_base.go`
- `internal/server/handlers/repo_sha_seed.go`
- `internal/server/handlers/nodes_claim_response.go`
- `internal/store/queries/run_repos.sql`
- `internal/worker/hydration/git_fetcher.go`
- `internal/workflow/step/diff.go`
- `internal/nodeagent/git/repo_sha_v1.go`
