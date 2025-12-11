Descriptor HTTPS Quickstart

Goal
- Configure the workstation’s cached descriptor for HTTPS-only operation with TLS verification and endpoint failover.

Inputs
- Cluster CA bundle: `ca.pem`
- API endpoints (IPs or hostnames with port): `https://203.0.113.10:8443`, `https://203.0.113.11:8443`
- SNI/server name to verify: `api.<cluster-id>.ploy`
 

Command
```bash
dist/ploy cluster https \
  --cluster-id alpha \
  --api-endpoint https://203.0.113.10:8443 \
  --api-endpoint https://203.0.113.11:8443 \
  --api-server-name api.alpha.ploy \
  --ca-file ./ca.pem \
  --disable-ssh
```

What it does
- Saves api_endpoints[] and api_server_name into `~/.config/ploy/clusters/alpha.json`.
- Stores ca_bundle (PEM) and sets disable_ssh=true so the CLI uses HTTPS directly.
- The CLI now tries endpoints in order on network errors or 502/503/504 responses.

Verification
```bash
# Health
curl -sSI --cacert ca.pem https://api.alpha.ploy/v1/status | head -n1

# Optional: submit a run and attach an artifact bundle
# (Requires a valid run ID created via 'ploy mod run'; run IDs are KSUID-backed strings).
# (manual CLI artifact upload via `ploy upload` has been removed)
```

Rollback
- Re-run the command without `--disable-ssh` to leave SSH as a fallback:
  - `dist/ploy cluster https --cluster-id alpha --api-endpoint ... --ca-file ca.pem`
