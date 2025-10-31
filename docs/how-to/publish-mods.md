Publish Mods Images via CLI (HTTPS)

Overview
- Mods images live under `docker/mods/`:
  - `mod-openrewrite` — OpenRewrite apply (Maven)
  - `mod-llm` — LLM plan/execute stub
  - `mod-plan` — Planner stub
  - `mod-human` — Human gate stub
- The runner pulls images from `registry.<cluster-id>.ploy/ploy/<name>:latest` by default (configurable via `PLOY_REGISTRY_HOST`).
- Publish via the Ploy CLI registry group over HTTPS; no Docker login or SSH staging required.

Prerequisites
- A descriptor with HTTPS fields: `api_endpoints`, `api_server_name`, and `ca_bundle`.
- Registry TLS: workers trust the CA at `/etc/docker/certs.d/registry.<cluster-id>.ploy/ca.crt`.
- Set `PLOY_REGISTRY_HOST=registry.<cluster-id>.ploy` on the control plane (or as an env for the runner) to align job templates.

Quick setup (workstation)
```bash
dist/ploy cluster https \
  --cluster-id alpha \
  --api-endpoint https://203.0.113.10:8443 --api-endpoint https://203.0.113.11:8443 \
  --api-server-name api.alpha.ploy \
  --registry-host registry.alpha.ploy \
  --ca-file ./ca.pem --disable-ssh
```

Option A — Batch publish all Mods images
```bash
scripts/push-mods-via-cli.sh
# Discovers docker/mods subfolders, builds OCI layouts via docker buildx,
# then pushes blobs and puts :latest manifest using `ploy registry` commands.
```

Option B — Publish a single Mods image
```bash
# 1) Build OCI layout for a single context
name=mod-openrewrite
docker buildx build --platform linux/amd64 --output type=oci,dest=${name}.oci.tar docker/mods/${name}
mkdir -p /tmp/${name}.oci && tar -C /tmp/${name}.oci -xf ${name}.oci.tar

# 2) Push blobs via CLI (config + layers)
mf=/tmp/${name}.oci/blobs/sha256/$(jq -r '.manifests[0].digest' /tmp/${name}.oci/index.json | sed 's/^sha256://')
cfg_d=$(jq -r '.config.digest' "$mf"); cfg_p=/tmp/${name}.oci/blobs/sha256/${cfg_d#sha256:}
dist/ploy registry push-blob --repo ploy/mods-openrewrite --media-type $(jq -r '.config.mediaType' "$mf") "$cfg_p"
for i in $(jq -r '.layers[].digest' "$mf"); do 
  p=/tmp/${name}.oci/blobs/sha256/${i#sha256:}
  mt=$(jq -r --arg d "$i" '.layers[]|select(.digest==$d).mediaType' "$mf")
  dist/ploy registry push-blob --repo ploy/mods-openrewrite --media-type "$mt" "$p"
done

# 3) Put manifest as :latest
dist/ploy registry put-manifest --repo ploy/mods-openrewrite --reference latest "$mf"
```

Verify pulls from a node
```bash
docker pull registry.<cluster-id>.ploy/ploy/mods-openrewrite:latest
```

Notes
- Directory name to repo mapping: `mod-foo` (folder) corresponds to `ploy/mods-foo` (registry repo) to match runner templates.
- If you customize repo names, also adjust the runner templates or set `PLOY_REGISTRY_HOST` and image paths accordingly.

