# VPS Lab Environment

Integration and end-to-end tests for Ploy v2 should target the dedicated VPS lab. The cluster
consists of three nodes:

| Node | IP Address | SSH Access |
| --- | --- | --- |
| Node A | `45.9.42.212` | `root@45.9.42.212` |
| Node B | `46.173.16.177` | `root@46.173.16.177` |
| Node C | `81.200.119.187` | `root@81.200.119.187` |

Guidelines:

- Use these hosts for integration/E2E testing.
- Bootstrap beacon nodes with `dist/ploy deploy bootstrap --address <ip>` and capture output for runbooks.
- Bootstrap the IPFS Cluster lab by running
  `scripts/ipfs/bootstrap_lab_cluster.sh up --ssh-host root@45.9.42.212` (or the desired host) from
  your workstation; the script copies compose assets, installs Docker/compose on the VPS if missing,
  and executes Docker remotely via SSH.
- Keep SSH keys restricted to the lab team and rotate credentials regularly.
- Clean up temporary artifacts after test runs (IPFS pins, etcd keys, Docker containers) to avoid
  state drift between test cycles. Tear down the cluster with
  `scripts/ipfs/bootstrap_lab_cluster.sh down --destroy-data`.
