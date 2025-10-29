# Update a Ploy Cluster

This guide outlines how to update `ployd` to a new version on the VPS lab or any Ploy node. All
nodes are equivalent: each serves control APIs and can execute jobs. Update all nodes you want
participating in execution.

## 1) Build ployd for Linux

Option A (recommended): use Makefile targets

```bash
make build            # produces dist/ployd and dist/ployd-linux
```

Option B: direct Go build

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags='-s -w' -o dist/ployd-linux ./cmd/ployd
```

Optional: verify version locally

```bash
./dist/ployd-linux --version
```

## 2) Copy binary to the VPS lab

Upload to each node, then atomically replace the existing binary:

```bash
NODE_IPS=(45.9.42.212 46.173.16.177 81.200.119.187)
for ip in "${NODE_IPS[@]}"; do
  scp -q dist/ployd-linux root@"$ip":/usr/local/bin/ployd.new
  ssh -q root@"$ip" 'install -m 0755 /usr/local/bin/ployd.new /usr/local/bin/ployd && rm -f /usr/local/bin/ployd.new'
done
```

If not using `root`, prefix commands with `sudo` and ensure your user has permission to write under `/usr/local/bin`.

## 3) Restart the service

```bash
for ip in "${NODE_IPS[@]}"; do
  ssh -q root@"$ip" 'systemctl restart ployd && systemctl is-active --quiet ployd'
done
```

## 4) Sanity check the rollout

- Service status and recent logs:

```bash
ssh root@45.9.42.212 'systemctl status --no-pager ployd'
ssh root@45.9.42.212 'journalctl -u ployd -n 50 --no-pager'
```

- Control-plane `/v1/version` (via cached descriptor or direct IP):

```bash
# using direct IP over HTTPS if reachable from your host
curl -sk https://45.9.42.212:8443/v1/version | jq .

# or via the CLI with tunnels (descriptor under ~/.config/ploy/clusters)
./dist/ploy version
```

Repeat checks for the other nodes (`46.173.16.177`, `81.200.119.187`).

Tip: roll one node first, validate, then continue with the remaining nodes.
