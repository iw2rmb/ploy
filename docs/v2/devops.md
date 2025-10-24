# Deployment & Operations Guide

This guide describes how to bootstrap a Ploy v2 cluster and add additional
`ployd` nodes.
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
   - Create `ploy` user with sudo rights and `/var/lib/ploy` workspace for the
     `ployd` service runtime.

2. **Install Dependencies**  
   - Install Docker, etcd, and the IPFS Cluster daemon so `ployd` can launch
     jobs locally.  
   - Configure systemd units for etcd, IPFS Cluster, and prepare the `ployd`
     service definition.

3. **Run Bootstrap Script via CLI**  
   - Execute `dist/ploy deploy bootstrap` so the CLI can prepare the host and wire trust material.
     The command automatically generates a 16-character lowercase-hex cluster identifier (persisted
     locally as the default descriptor) and a 4-character node identifier reused for the initial
     beacon/worker registration.
   - `--etcd-endpoints <http(s)://host:port>[,<...>]` — optional override for etcd endpoints reachable
     from the operator workstation. When omitted the CLI assumes `http://<address>:2379` (or the
     derived host) after bootstrap.
   - `--beacon-url <https://beacon.example.com>` — optional override for the beacon discovery URL.
     Defaults to `https://<node-id>.<cluster-id>.ploy` so clients resolve through the beacon DNS.
   - The CLI generates and stores a cluster API key locally for later `ploy cluster connect` calls.
   - The bootstrap command automatically registers the generated node as both beacon and worker
     metadata, exposing its `ployd` endpoint as `<node-id>.<cluster-id>.ploy`.
  - The CLI uploads the `ployd` binary (defaults to the executable found alongside the CLI; override with `--ployd-binary <path>`) and then streams the embedded bootstrap shell script over SSH. The script converges dependencies, writes the initial `/etc/ploy/ployd.yaml`, and installs the systemd unit.
  - Once the script completes, the CLI verifies `etcd` and `ployd` are active via `systemctl` before continuing. `ployd` starts in beacon mode immediately so the node advertises discovery endpoints.
   - The command runs preflight checks (package manager, disk at `${PLOY_WORKDIR:-/var/lib/ploy}`,
     and port availability) before installing Go 1.25.2, etcd 3.6.0, Docker 28.0.1, and IPFS
     Cluster 1.1.4.  
   - The CLI always uses `<cluster-id>.ploy` so the beacon domain matches the generated identifier; configure DNS separately when a fixed hostname is required.  
   - After the remote script succeeds, the CLI elevates via `sudo` to install the cluster CA into
     the workstation trust store. On macOS it also checks `/etc/resolver/ploy`; when the file is
     missing it prompts to write a resolver entry pointing `*.ploy` lookups to the beacon IP.
   - Beacon nodes are regular workers operating in beacon mode. The generated node ID is reused for
     both the beacon and worker records so the beacon DNS can resolve `*.{cluster-id}.ploy` and
     `<node-id>.<cluster-id>.ploy` entries for the same machine.
   - SSH defaults to `root@<address>` with identity file `~/.ssh/id_rsa`.  
   - The minimum disk check defaults to 4 GiB and is enforced automatically; ensure hosts satisfy it before running the bootstrap.
   - All binaries are pinned via static downloads inside `/usr/local/bin`, systemd units are
     refreshed, and logs summarise installed versions.  
   - etcd is installed as a systemd service bound to `127.0.0.1:{2379,2380}` so the CLI can finish
     bootstrap writes without exposing client ports publicly.
   - The script confines temporary files to `${PLOY_WORKDIR}` and ensures Docker is enabled with
     a sane `daemon.json` default.
   - Example command with overrides:

     ```bash
     dist/ploy deploy bootstrap \
       --beacon-url https://beacon.staging.example.com \
       --address 45.9.42.212 \
       --user root \
       --identity ~/.ssh/ploy-lab
     ```

4. **Capture Cluster Metadata & PKI**  
   - On success the CLI invokes the deployment PKI manager (see
     [`.archive/deployment-pki-bootstrap/README.md`](../../.archive/deployment-pki-bootstrap/README.md)),
     generating the cluster CA plus beacon and worker leaf certificates. Material is stored in etcd under
     `/ploy/clusters/<cluster>/security/...` with revocation markers and per-node descriptors that the worker
     onboarding flow consumes.
   - The trust bundle is published via the control-plane security store so subsequent `ploy cluster
     connect` calls download the latest CA chain automatically.
   - The CLI writes the CA bundle to `${XDG_CONFIG_HOME}/ploy/clusters/<id>_ca.pem` and persists a
     cluster descriptor alongside it (`<id>.json`). The descriptor records the beacon URL, optional
     control-plane endpoint, API key, CA bundle path, and the active CA version returned by the PKI
     manager.
   - With the CA materials present, trust-sensitive commands such as `ploy node add --dry-run` and
     `ploy beacon rotate-ca --dry-run` succeed immediately after bootstrap, enabling smoke tests
     against the lab cluster before onboarding additional nodes.
   - Enables fast reconnection via `ploy cluster connect --cluster-id <id>` or `ploy cluster list`
     once the descriptor exists; subsequent commands reuse the stored CA bundle and API key. If the CA
     rotates, the descriptor is refreshed automatically during the rotation workflow.

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
  - Run `ploy node add --address <host-or-ip>` and include any metadata labels with `--label key=value`. Use `--user`, `--identity`, `--ssh-port`, or `--ployd-binary` if the defaults (`root`, `~/.ssh/id_rsa`, `22`, CLI-adjacent `ployd`) are unsuitable.  
   - The CLI derives the target cluster from the default cached descriptor (created during bootstrap)
     and generates a 4-character worker identifier automatically.
   - Provide at least one health endpoint using `--health-probe name=https://<addr>:9443/healthz`; multiple probes are allowed.  
   - TLS health probes presenting certificates issued by the deployment CA are trusted automatically during onboarding.  
  - The CLI first SSHes into the worker, uploads `ployd`, reruns the bootstrap script with `PLOYD_MODE=worker`, and verifies the `ployd` service is active. It then calls the beacon control-plane (`/v2/nodes`) to write worker descriptors into etcd (`/ploy/clusters/<cluster>/registry/workers/<id>`), issue a worker certificate via the deployment CA manager, and record probe outcomes.  
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

This operational flow keeps `ployd` nodes consistent and ensures the control plane remains
authoritative via etcd and beacon mode.

## Certificate Lifecycle

- Deployment bootstrap generates the initial cluster CA and issues leaf certificates for the beacon
  plus any pre-registered workers. The materials live under `/ploy/clusters/<cluster>/security/` in etcd
  alongside a trust bundle exposed to CLI consumers.
   - Subsequent worker onboarding reuses these descriptors; the onboarding flow in
     [`.archive/deployment-worker-onboarding/README.md`](../../.archive/deployment-worker-onboarding/README.md)
     reads the same paths to provision node keys.
- Run `ploy beacon rotate-ca --cluster-id <id>` to mint a new CA, publish refreshed leaf certificates,
  and mark the previous CA version as revoked under `/security/ca/history/<version>`. Use
  `--dry-run` to stage the rotation before writing to etcd.
- After rotation, the trust store and local cluster descriptors are updated automatically so tools and
  operators download the new CA bundle on their next refresh.
