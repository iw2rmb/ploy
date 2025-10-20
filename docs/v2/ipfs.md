# IPFS Integration

Ploy v2 treats IPFS Cluster as the primary artifact store for snapshots, diffs, build gate reports,
and logs. Every `ploynode` runs a local IPFS daemon and joins the cluster so artifacts remain
available regardless of which node produced them.

## Topology

- **Per-node daemon** — Each worker (including the beacon node) runs an IPFS daemon and IPFS Cluster
  service. Nodes join the cluster using shared credentials distributed during bootstrap.
- **etcd coordination** — Cluster peer information (peer IDs, multiaddrs) is stored in etcd under
  `ipfs/peers/*` so new nodes can discover existing members.
- **Replication factor** — Set the desired pin replication count (default 2) in the cluster config.
  This ensures that new artifacts are pinned on multiple nodes.

## Data Paths

- **Runtime nodes** — Upload artifacts (diffs, build gate reports, logs) directly to their local
  IPFS Cluster API, minimising control-plane bandwidth. Only the resulting CID and metadata are
  written to etcd.
- **Workstation/CLI** — When operators need to upload artifacts manually, the control plane exposes
  `POST /v2/artifacts/upload`, which proxies the payload into IPFS Cluster. This keeps credentials
  centralised while still producing the same CID workflow.

## Bootstrapping

1. During `ploy deploy bootstrap`, the CLI installs the IPFS Cluster daemon on the beacon host and
   generates cluster secrets.
2. `ploy node add` downloads those secrets, installs the daemon, and joins the cluster automatically.
3. Each node reports its IPFS peer ID and status back to etcd so the control plane can monitor health.

## Pinning and Garbage Collection

- `ploynode` pins all new artifact CIDs (diff bundles, build gate reports) once they are uploaded.
- The cluster enforces the replication factor; nodes receiving pins publish status back to etcd.
- Garbage collection is coordinated by the control plane:
  - Artifacts marked for deletion are unpinned via the IPFS Cluster REST API.
  - Nodes run `ipfs repo gc` on a schedule once unpin operations complete.
- Operators can trigger clean-up using `ploy artifact gc`, which invokes the same unpin/GC workflow.

## Failure Handling

- Because artifacts replicate across multiple nodes, a single node loss does not impact availability.
- When a node rejoins, it syncs pins using IPFS Cluster’s consensus (RAFT by default).
- If quorum is lost, run the cluster recovery procedure documented in `docs/v2/devops.md`.

## References

- `docs/v2/devops.md` — Deployment instructions covering IPFS Cluster installation on beacon and
  worker nodes.
- `docs/v2/job.md` — Describes how job outcomes upload artifacts to IPFS and record the resulting CIDs.
- `docs/design/ipfs-artifacts/README.md` — Background design notes on IPFS usage within Ploy.
