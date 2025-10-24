# IPFS Cluster Integration

Ploy Next replaces the workstation filesystem artifact cache with an IPFS Cluster
backed store. Every step execution publishes diff bundles, log archives, and
auxiliary assets directly to the cluster so any node can hydrate artifacts by
CID.

## Artifact Pipeline

1. The step runtime captures diff tarballs from the writable workspace mount
   and streams them to the cluster via the embedded client in
   `internal/workflow/artifacts`.
2. Log buffers are uploaded using the same pathway. Both uploads compute a
   SHA-256 digest before transmission so the resulting CID and digest can be
   recorded in workflow checkpoints.
3. Replication factors default to the workstation configuration:
   - `PLOY_IPFS_CLUSTER_REPL_MIN`
   - `PLOY_IPFS_CLUSTER_REPL_MAX`
   Operators can override these values per upload via CLI flags when testing.
4. Additional metadata (artifact name, kind) is stored with each pin to aid
   debugging and operational tooling.

## CLI Commands

The CLI routes artifact management commands to the cluster client:

- `ploy artifact push [--name <name>] [--kind <kind>] <path>` uploads an
  artifact and prints the CID, digest, size, and replication settings.
- `ploy artifact pull <cid> [--output <path>]` downloads and optionally writes
  the artifact to disk, reporting the digest for verification.
- `ploy artifact status <cid>` reports peer pin states and replication
  thresholds to help operators detect skew.
- `ploy artifact rm <cid>` initiates an unpin request when an artifact is no
  longer required.

## Operational Guidance

- Ensure `PLOY_IPFS_CLUSTER_API` points at an IPFS Cluster peer reachable from
  the workstation. Authentication can be provided via
  `PLOY_IPFS_CLUSTER_TOKEN` (bearer) or
  `PLOY_IPFS_CLUSTER_USERNAME`/`PLOY_IPFS_CLUSTER_PASSWORD` (basic auth).
- The `tests/integration/artifacts` suite exercises the client behaviour against
  a mocked cluster API. Extend this coverage as real cluster endpoints become
  available.
- When diagnosing replication issues, start with `ploy artifact status` to see
  which peers report lagging pins. Combine with cluster daemon logs to isolate
  connectivity or disk pressure problems.
