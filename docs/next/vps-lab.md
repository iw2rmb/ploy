# VPS Lab Environment

Integration and end-to-end tests for Ploy Next should target the dedicated VPS lab. The cluster
consists of three nodes:

| Node | IP Address | SSH Access |
| --- | --- | --- |
| Node A | `45.9.42.212` | `root@45.9.42.212` |
| Node B | `46.173.16.177` | `root@46.173.16.177` |
| Node C | `81.200.119.187` | `root@81.200.119.187` |

Guidelines:

- Use these hosts for integration/E2E testing.
- Bootstrap cluster nodes with `dist/ploy cluster add --address <ip>` (omit `--cluster-id` on the first node) and capture output for runbooks, including the descriptor join hint printed at the end. Always connect as `root`; the CLI reuses the exact SSH identity for both the SSH session and the SCP upload so no `PLOY_SSH_ADMIN_KEYS_B64` payload is required.
- After the first host converges, `ployd` auto-generates the cluster CA. Use `ploy cluster cert status` to confirm the active CA and expiry before onboarding additional workers.
- For full cluster bootstrap and dependency setup, follow
  [docs/how-to/deploy-a-cluster.md](../how-to/deploy-a-cluster.md). This replaces earlier ad‑hoc IPFS
  bootstrap scripts.
- Keep SSH keys restricted to the lab team and rotate credentials regularly.
- Clean up temporary artifacts after test runs (IPFS pins, etcd keys, Docker containers) to avoid
  state drift between test cycles.
- Expose the control plane on the lab network so operators can poll `/v1/status` for queue depth and
  worker readiness, `/v1/config` for configuration audits, and `/v1/nodes` (over an SSH tunnel) to
  verify worker inventory. Capture the responses in incident reports so they can be replayed offline.
