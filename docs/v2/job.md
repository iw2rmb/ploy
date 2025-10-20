# Job Execution Model

Ploy v2 executes every step—Mods, build gate stages, auxiliary tooling—through a consistent job
abstraction. This mirrors the Grid runtime semantics so workstation workflows remain predictable.

## Job Types

- **Mod step jobs** — Run the units defined in a Mod plan (plan, apply, healing actions). These jobs
  are associated with a Mod ticket and appear both under `/v2/mods/{ticket}` and `/v2/jobs/{id}`.
- **Build gate jobs** — Execute the SHIFT build gate (sandbox + static checks). They share the same
  schema but are flagged with `type: buildgate` in the metadata.
- Additional auxiliary jobs (e.g., log ingestion) can reuse the same pattern if future roadmap items
  require them.

## Lifecycle

1. **Submission** — The control plane creates a job record in etcd, capturing image, command,
   environment variables, mounts, and metadata (ticket, step ID, requester).
2. **Execution** — The workflow runner materialises the inputs (repo snapshots, cumulative diffs,
   secrets) through the job service, mounts the hydrated workspace into the container, and launches
   it via the Docker runtime adapter.
3. **Monitoring** — Runtime state (queued, running, succeeded, failed, timed out) and resource usage
   are persisted back to etcd. Log metadata (CID, digest, tail snippet) is recorded while the full
   log payload lives in IPFS (see [docs/v2/logs.md](logs.md)).
4. **Retention** — Job records, stdout/stderr, and structured metadata are retained according to
   policy (default seven days) for audit and debugging.

## Container Handling

- Containers are launched with `auto-remove` disabled so logs and exit metadata can be collected.
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

- Diff-producing steps (e.g., ORW apply) upload a tarball or patch bundle to IPFS Cluster using the
  node’s local cluster client, keyed by the job ID. The returned CID, size, and checksum are
  persisted in the job outcome stored in etcd.
- Ploy nodes compute diffs after each step by comparing the hydrated workspace against the baseline
  tree. The resulting bundle is uploaded to IPFS Cluster, keyed by the job ID, and recorded in etcd
  with CID, size, and checksum.
- Build gate runs emit structured JSON reports (errors, static check findings) into IPFS. The job
  metadata links the report CID alongside the log digest, status, and failure reason.
- Job metadata lives under `mods/<ticket>/jobs/<job-id>` in etcd, providing a compact index so the
  control plane and CLI can resolve artifacts without downloading full payloads until needed.

## Failure Semantics

- Failures capture exit code, reason, and the tail of stdout/stderr.
- Build gate failures trigger healing workflows (e.g., `llm-plan`), while Mods mark the offending
  step for operator review.
- Retries create new job records linked to the original ticket for traceability.
- Operator-driven resumes (via `ploy mod resume`) are distinct: they rehydrate the Mod from stored
  artifacts and enqueue fresh jobs only when administrators explicitly request it. Automated retries
  respect the per-job `retry` budget.

## CLI & API Touchpoints

- `ploy status` and `ploy mod inspect` surface job history for each step, with links to log streams
  and IPFS CIDs.
- `ploy logs job <job-id>` tails logs via SSE and fetches archived bundles from IPFS when complete.
- API consumers can query `GET /v2/jobs/{id}` for status snapshots and `GET /v2/jobs/{id}/logs` for
  archived output.
- Responses include the executing node ID so operators can reach the exact worker for further
  diagnostics (`ploy node logs` or node API).
