# Job Execution Model

Ploy v2 executes every step—Mods, build gate stages, auxiliary tooling—through a consistent job
abstraction. This mirrors the Grid runtime semantics so workstation workflows remain predictable.

## Job Types

- **Mod step jobs** — Run the units defined in a Mod plan (plan, apply, healing actions). Each job
  executes a single step manifest locally on the worker node and appears both under
  `/v2/mods/{ticket}` and `/v2/jobs/{id}`.
- **Build gate jobs** — Execute the SHIFT build gate (sandbox + static checks). They share the same
  schema but are flagged with `type: buildgate` in the metadata.
- Additional auxiliary jobs (e.g., log ingestion) can reuse the same pattern if future roadmap items
  require them.

## Lifecycle

1. **Submission** — The control plane persists a job record in etcd with state `queued`, priority,
   retry budget, and metadata, then inserts a queue entry under `queue/<kind>/<priority>/<job-id>`.
2. **Execution** — Worker nodes claim jobs through the scheduler HTTP API. Successful claims update
   the job to `running`, stamp `claimed_by`, attach an etcd lease, and delete the queue entry. The
   node hydrates the workspace (snapshot + cumulative diffs) and runs the step manifest inside a
   retained Docker container.
3. **Monitoring** — Runtime state transitions (`queued`, `running`, `succeeded`, `failed`,
   `inspection_ready`) plus timestamps, artifacts, and error metadata are persisted back to etcd.
   Lease heartbeats update the expiry timestamp. Log metadata (CID, digest, tail snippet) is
   recorded while the full payload lives in IPFS (see [docs/v2/logs.md](logs.md)).
4. **Retention** — Job records, stdout/stderr, and structured metadata are retained according to
   policy (default seven days) for audit and debugging. Terminal jobs also gain a GC marker under
   `gc/jobs/<job-id>` with an expiry timestamp for the retention controller.

## Container Handling

- Containers are launched with `auto-remove` disabled so logs and exit metadata can be collected,
  matching the local runtime defaults.
  Once log bundles are archived to IPFS and metadata is persisted, `ploynode` explicitly removes the
  container to avoid disk bloat.
- Nodes periodically clean up terminated containers once logs are archived and retention windows
  expire.
- Secrets are injected via temporary volumes or environment variables sourced from etcd; they are
  scrubbed immediately after job completion.

## Log Streaming

- Each job exposes a server-sent events stream at `GET /v2/jobs/{id}/logs/stream`. Events include
  incremental stdout/stderr chunks with timestamps.
- The same stream is referenced from Mod/build-gate summaries, allowing the CLI to tail logs in near
  real time.
- Logs are also persisted in IPFS Cluster (optional) for long-term storage, keyed by job ID.

## Outputs & Artifacts

- Diff-producing steps (e.g., ORW apply) package changes as deterministic tarballs generated from the
  writable workspace mount. The tarball is staged locally (hashed to produce a CID) and recorded in
  the job outcome stored in etcd. Subsequent tasks publish the staged artifacts to IPFS Cluster.
- Ploy nodes compute diffs after each step by comparing the hydrated workspace against the baseline
  tree. The resulting tarball is retained alongside the log bundle so the artifact publisher can push
  them to IPFS Cluster once the dedicated artifact store slice lands.
- Build gate runs emit structured JSON reports (errors, static check findings) into IPFS. The job
  metadata links the report CID alongside the log digest, status, and failure reason.
- Job metadata lives under `mods/<ticket>/jobs/<job-id>` in etcd, providing a compact index so the
  control plane and CLI can resolve artifacts without downloading full payloads until needed.

## Failure Semantics

- Failures capture exit code, reason, and the tail of stdout/stderr. Scheduler-induced failures
  (lease expiry, heartbeat timeout) set `error.reason = lease_expired` and honour the retry budget.
- Build gate failures continue to trigger healing workflows (e.g., `llm-plan`), while Mods mark the
  offending step for operator review.
- Retries increment `retry_attempt` on the same job record and re-enqueue the job when budget
  remains. Once exhausted, the job transitions to `failed` and a GC marker is written.
- Operator-driven resumes (via `ploy mod resume`) rehydrate the Mod from stored artifacts and
  enqueue fresh jobs only when administrators explicitly request it.

## CLI & API Touchpoints

- `POST /v2/jobs` submits work, `POST /v2/jobs/claim` lets workers compete for steps,
  `POST /v2/jobs/{id}/heartbeat` renews leases, and `POST /v2/jobs/{id}/complete` records terminal
  states.
- `GET /v2/jobs/{id}` and `GET /v2/jobs?ticket=` back the CLI (`ploy status`, `ploy mod inspect`)
  with complete lifecycle snapshots including lease metadata.
- `ploy logs job <job-id>` tails logs via SSE and fetches archived bundles from IPFS when complete.
- Responses include the executing node ID so operators can reach the exact worker for further
  diagnostics (`ploy node logs` or node API).
