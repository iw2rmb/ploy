# Deployment & Operations Guide

This guide describes how to bootstrap a Ploy v2 cluster and add additional nodes.
It assumes Linux hosts (VPS or bare metal) with SSH access.

## Prerequisites

- SSH access to all hosts with sudo privileges.
- Go 1.25.2 installed for building binaries.
- Docker Engine 28.0.1 (deployed via static binaries).
- IPFS Cluster service 1.1.4 installed.
- etcd 3.6.0 binary or package available.

## Bootstrap Steps (Beacon + Control Plane)

1. **Provision Host**  
   - Ensure OS packages are up to date.  
   - Create `ploy` user with sudo rights and `/var/lib/ploy` workspace.

2. **Install Dependencies**  
   - Install Docker, etcd, IPFS Cluster daemon.  
   - Configure systemd units for etcd and IPFS Cluster services.

3. **Run Bootstrap Script via CLI**  
   - Execute `dist/ploy deploy bootstrap --address 45.9.42.212`
     (swap the address for Node B/C as needed) and tee output to a log for troubleshooting.  
   - Use `--dry-run` to preview the embedded script (`internal/deploy/assets/bootstrap.sh`) before shipping it over SSH.  
   - The command runs preflight checks (package manager, disk at `${PLOY_WORKDIR:-/var/lib/ploy}`,
     and port availability) before installing Go 1.25.2, etcd 3.6.0, Docker 28.0.1, and IPFS
     Cluster 1.1.4.  
   - If `--host` is omitted, the CLI generates a beacon domain in the form `<16 hex chars>.ploy`
     using go-nanoid; override it explicitly when a fixed hostname is required.  
   - SSH defaults to `root@<address>` with identity file `~/.ssh/id_rsa`.  
   - The minimum disk check defaults to 4 GiB and is enforced automatically; ensure hosts satisfy it before running the bootstrap.
   - All binaries are pinned via static downloads inside `/usr/local/bin`, systemd units are
     refreshed, and logs summarise installed versions.  
   - The script confines temporary files to `${PLOY_WORKDIR}` and ensures Docker is enabled with
     a sane `daemon.json` default.
   - Example command with overrides:

     ```bash
     dist/ploy deploy bootstrap \
       --address 45.9.42.212 \
       --host beacon-lab.ploy \
       --user root \
       --identity ~/.ssh/ploy-lab
     ```

4. **Capture Cluster Metadata & PKI**  
   - On success the CLI invokes the deployment PKI manager, generating the cluster CA plus beacon and
     worker leaf certificates. Material is stored in etcd under `/ploy/clusters/<cluster>/security/...`
     with revocation markers and per-node descriptors that the worker onboarding flow consumes.
   - The trust bundle is published via the control-plane security store so subsequent `ploy cluster
     connect` calls download the latest CA chain automatically.
   - The CLI also writes a cluster descriptor (beacon address, API key, CA path) under
     `${XDG_CONFIG_HOME}/ploy/clusters/<id>.json`, including the cluster version returned by
     `GET /v2/version` to detect drift.
   - Enables fast reconnection via `ploy cluster connect --beacon-ip <ip> --api-key <key>` when joining
     an existing deployment; version mismatches trigger a metadata refresh.

5. **Configure Ploy CLI**  
   - Install `ploy` binary on operator workstation.  
   - Set environment variables (`PLOY_BEACON_URL`, `PLOY_CA_PATH`, GitLab API token in etcd).  
   - Run `ploy beacon promote` if beacon rotation is required.

6. **Verify**  
   - `ploy beacon status` (or API equivalent) to confirm healthy nodes list.  
   - `ploy status` to ensure etcd, IPFS, and Docker integrations respond.

## Adding a Worker Node

1. **Provision Host**  
   - Create `ploy` user and workspace.  
   - Install Docker, IPFS Cluster client, and join the cluster (pinning mirror).  
   - Install etcd client tools if needed.

2. **Deploy Runtime via CLI**  
   - Run `ploy node add --cluster-id <cluster> --worker-id <worker-id> --address <host-or-ip>` and include any metadata labels with `--label key=value`.  
   - Provide at least one health endpoint using `--health-probe name=https://<addr>:9443/healthz`; multiple probes are allowed.  
   - The CLI writes worker descriptors into etcd (`/ploy/clusters/<cluster>/registry/workers/<id>`), issues a worker certificate via the deployment CA manager, and records probe outcomes.  
   - Use `--dry-run` to preview probes and certificate issuance without modifying etcd. Successful runs store the PEM bundle for the worker under the security prefix and surface the certificate version in the CLI output.  
   - Confirm the worker fetches its materials at `/etc/ploy/pki/` and registers with the beacon services.

3. **Validation**  
   - `etcdctl get /ploy/clusters/<cluster>/registry/workers --prefix --keys-only` to confirm the descriptor exists.  
   - Run a smoke Mod to confirm job submission, log streaming, and artifact uploads.

## Maintenance

- Use `ploy beacon rotate-ca --cluster-id <id> --dry-run` to preview CA changes, then rerun without
  `--dry-run` to rotate the CA, reissue beacon/worker certificates, update the trust bundle, and record
  revocation metadata in etcd. Worker onboarding reads the refreshed descriptors automatically.  
- Use `ploy logs job <job-id>` for debugging, and clean up old containers using node operations.  
- Monitor etcd health (`etcdctl endpoint status`) and IPFS Cluster pinning status regularly.
- Use `ploy config gitlab rotate --secret <name> --api-key <token> --scope <scope>` to push new GitLab credentials through the signer. The command talks to the control plane, writes the encrypted secret, and emits rotation events so workers refresh immediately.  
- Inspect signer health with `ploy config gitlab status [--secret <name>]`. The output includes audit feed metadata from the rotation revocation pipeline (last rotation, revoked nodes, recent failures) outlined in `.archive/gitlab-rotation-revocation/README.md`.  
- Stream `ploy jobs follow <job-id>` when closing out incidents; the final `Retention:` line echoes the job’s bundle CID, TTL, and expiry so teams can schedule inspections before GC removes the log bundle (see [docs/v2/logs.md](logs.md)).  
- For unattended rotations, provide the control-plane base URL via `PLOY_CONTROL_PLANE_URL` or ensure the active cluster descriptor contains the control plane endpoint and CA bundle so the CLI can authenticate requests.

This operational flow keeps Ploy nodes consistent and ensures the control plane remains
authoritative via etcd and beacon mode.

## Certificate Lifecycle

- Deployment bootstrap generates the initial cluster CA and issues leaf certificates for the beacon
  plus any pre-registered workers. The materials live under `/ploy/clusters/<cluster>/security/` in etcd
  alongside a trust bundle exposed to CLI consumers.
- Subsequent worker onboarding reuses these descriptors; the onboarding flow in
  `docs/design/deployment-worker-onboarding/README.md` reads the same paths to provision node keys.
- Run `ploy beacon rotate-ca --cluster-id <id>` to mint a new CA, publish refreshed leaf certificates,
  and mark the previous CA version as revoked under `/security/ca/history/<version>`. Use
  `--dry-run` to stage the rotation before writing to etcd.
- After rotation, the trust store and local cluster descriptors are updated automatically so tools and
  operators download the new CA bundle on their next refresh.
