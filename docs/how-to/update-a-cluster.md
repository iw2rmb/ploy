# Update a Ploy Cluster (Server/Node Architecture)

This guide aligns with the SIMPLE.md pivot: a single control‑plane server binary (`ployd`) and
one or more worker nodes (`ployd-node`). The server hosts the API/scheduler/PostgreSQL; nodes
execute jobs and communicate with the server over mTLS. The VPS lab layout we use:

- Server (A): 45.9.42.212
- Node (B):   46.173.16.177
- Node (C):   81.200.119.187

Update order: update the control‑plane server first, then roll the worker nodes.

## 1) Build Binaries

Use the Makefile to build the CLI and daemons:

```bash
make build
# Produces at least:
#   dist/ploy                  # CLI (runs on your workstation)
#   dist/ployd                 # server (if cmd/ployd present)
#   dist/ployd-linux           # server (linux)
#   dist/ployd-node            # worker agent
#   dist/ployd-node-linux      # worker agent (linux)
```

Optional: verify versions locally

```bash
./dist/ployd-linux --version || true
./dist/ployd-node-linux --version || true
```

## 2) Update the Control‑Plane Server (A)

Do not re‑run `ploy server deploy` for an update (that regenerates PKI). Copy the new
binary and restart the service.

```bash
scp -q dist/ployd-linux root@45.9.42.212:/usr/local/bin/ployd.new
ssh -q root@45.9.42.212 'install -m 0755 /usr/local/bin/ployd.new /usr/local/bin/ployd && rm -f /usr/local/bin/ployd.new && systemctl restart ployd && systemctl is-active --quiet ployd'
```

Sanity checks:

```bash
ssh root@45.9.42.212 'systemctl status --no-pager ployd'
ssh root@45.9.42.212 'journalctl -u ployd -n 50 --no-pager'
curl -sk https://45.9.42.212:8443/v1/version | jq .
```

## 3) Update Worker Nodes (B, C)

Copy the new node agent binary to each worker and restart its service.

```bash
for ip in 46.173.16.177 81.200.119.187; do
  scp -q dist/ployd-node-linux root@"$ip":/usr/local/bin/ployd-node.new
  ssh -q root@"$ip" 'install -m 0755 /usr/local/bin/ployd-node.new /usr/local/bin/ployd-node && rm -f /usr/local/bin/ployd-node.new && systemctl restart ployd-node && systemctl is-active --quiet ployd-node'
done
```

Sanity checks per node:

```bash
ssh root@46.173.16.177 'systemctl status --no-pager ployd-node'; ssh root@46.173.16.177 'journalctl -u ployd-node -n 50 --no-pager'
ssh root@81.200.119.187 'systemctl status --no-pager ployd-node'; ssh root@81.200.119.187 'journalctl -u ployd-node -n 50 --no-pager'
```

## 4) Cluster Verification

- Descriptor: confirm your CLI points at the updated server

```bash
cat ~/.config/ploy/clusters/default          # shows current cluster-id
cat ~/.config/ploy/clusters/<cluster-id>.json
```

- Submit a quick Mods run and follow logs:

```bash
./dist/ploy mod run \
  --repo-url https://github.com/example/repo.git \
  --repo-base-ref main \
  --repo-target-ref feature/update-rollout \
  --follow
```

- Check jobs/events if needed:

```bash
./dist/ploy jobs ls --ticket <ticket-id>
./dist/ploy jobs follow <job-id>
```

## Rollback Tips

- Keep a backup of the previous binaries on each host (e.g., `/usr/local/bin/ployd.prev`, `/usr/local/bin/ployd-node.prev`).
- To rollback, swap the symlink or `install` the previous file and restart the corresponding service.

## Notes

- mTLS only: no SSH tunnels or bearer tokens are used at runtime. The CLI reads endpoint and CA bundle
  from the cluster descriptor under `~/.config/ploy/clusters/` (see `docs/envs/README.md`).
- PostgreSQL remains untouched during a binary update. If your server uses a local Postgres installed by
  the bootstrap, ensure the service is healthy before restarting `ployd`.
- Prefer rolling nodes one at a time to keep capacity available during updates.
