HTTPS End‑to‑End: Verify v2 APIs and Registry

Prerequisites
- Cluster CA + server certs deployed for:
  - api.<cluster-id>.ploy (control plane)
  - registry.<cluster-id>.ploy (registry v2)
- Workstation trusts CA (save to ca.pem) and has a valid token (Authorization header) if your deployment enforces it.
- Nodes trust the registry CA at `/etc/docker/certs.d/registry.<cluster-id>.ploy/ca.crt`.

1) Control Plane v2 health (registry alias)
```bash
curl -sSI --cacert ca.pem https://registry.<cluster-id>.ploy/v2/ | head -n1
# Expect: HTTP/2 200
```

2) Artifacts upload over HTTPS (no SSH)
```bash
# Create a tiny payload
echo 'hello-https' > payload.bin

# Compute sha256 (Linux or macOS)
digest="sha256:$( (sha256sum payload.bin 2>/dev/null || shasum -a 256 payload.bin) | awk '{print $1}')"

# Upload to /v2/artifacts/upload
curl -sS --cacert ca.pem \
  -X POST "https://api.<cluster-id>.ploy/v2/artifacts/upload?job_id=e2e-https&kind=report&digest=${digest}" \
  -H 'Content-Type: application/octet-stream' --data-binary @payload.bin | jq
```

3) Inspect and download artifact over HTTPS
```bash
artifact_id="<id from previous response>"
curl -sS --cacert ca.pem "https://api.<cluster-id>.ploy/v2/artifacts/${artifact_id}" | jq
curl -sS --cacert ca.pem -o download.bin "https://api.<cluster-id>.ploy/v2/artifacts/${artifact_id}?download=true"
```

4) Docker Registry v2 pull over TLS
```bash
# After publishing images via CLI (scripts/push-mods-via-cli.sh)
docker pull registry.<cluster-id>.ploy/ploy/mods-openrewrite:latest
```

Notes
- Pushing OCI blobs and manifests: use the `ploy registry` CLI commands. The server exposes standard v2 endpoints, but the current publish workflow stages blobs via the control plane.
- For worker pulls, set `PLOY_REGISTRY_HOST=registry.<cluster-id>.ploy` on the control plane (or as an env) so job templates resolve the correct host.

