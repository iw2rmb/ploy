# Ploy Features

## 🎯 Core Purpose
Maximum performance PaaS using unikernels, jails, and VMs with Heroku-like developer experience.

⸻

## 🛠 Build Lanes (A–F)

Auto-classified lanes:
- **Lane A** – Unikraft Minimal (Go, C)
  - KraftKit unikernel images
  - `<app>-<sha>.img` deterministic naming
  - SBOM + signature generation
- **Lane B** – Unikraft POSIX (Node, Python)
  - Musl libc POSIX layer
  - Dropbear SSH for debug (planned)
- **Lane C** – OSv Java/Scala
  - Jib → Capstan → `<app>-<sha>.qcow2`
  - Custom MainClass support
- **Lane D** – FreeBSD Jails
  - `<app>-<sha>-jail.tar` rootfs
  - Lightweight isolation for legacy apps
- **Lane E** – OCI + Kontain
  - `harbor.local/ploy/<app>:<sha>` images
  - `io.kontain` runtime for VM isolation
- **Lane F** – Full VMs
  - `<app>-<sha>.img` via Packer
  - Maximum compatibility fallback

⸻

## ⚙️ Builders
- Per-lane scripts in `build/` directory
- Auto SBOM (Syft) + signatures (Cosign)
- Deterministic `<app>-<sha>` naming
- Standalone or controller invocation

⸻

## 📦 Supply Chain Security
- SBOM generation (Syft), vulnerability scans (Grype), signing (Cosign)
- Storage upload to object storage (planned)
- OPA policy enforcement:
  - Requires signature + SBOM
  - SSH blocked in prod without break-glass
  - Image size caps per lane (planned)
  - Enhanced lane detection (Jib, C-extensions)

⸻

## 🚀 Deployment
- Nomad templates per lane in `platform/nomad/`
- Jobs include health checks, Vault integration, canary rollouts, Consul registration
- Controller handles rendering, submission, health polling

⸻

## 🌐 Routing & Preview
- Preview: `https://<sha>.<app>.ployd.app` triggers builds
- TTL cleanup for previews (planned)
- Domains: `manifests/<app>.yaml` configuration
- TLS: Certbot integration (planned), BYOC supported

⸻

## 👩‍💻 CLI (Go + Bubble Tea)
- `ploy apps new` – scaffold with /healthz
- `ploy push` – tar + stream to controller
- `ploy push --verify --diff` – verification branch testing (planned)
- `ploy open` – browser launch
- `ploy env` – manage app environment variables (planned)
- `ploy domains/certs/debug/rollback` – operations (planned)
- Workflow: push → build → deploy → open
- Self-healing loop support for LLM agents

⸻

## 🗄 Storage
- S3-compatible (MinIO default)
- Config: `configs/storage-config.yaml`
- Uploads: `artifacts/<app>/<sha>/`
- Backends: MinIO, Ceph, AWS S3

⸻

## 🔬 Sample Apps
`apps/` directory with Go, Node, Python, .NET, Scala, Java examples.
All include `/healthz` on port 8080.

⸻

## 🧪 CI/CD
- GitHub Actions: build, SBOM, scan, keyless sign
- GitLab CI: validate, build, supply-chain, deploy
- Artifact upload for traceability

⸻

## 🤖 Self-Healing Loop (planned)
- **Diff Push**: `POST /v1/apps/:app/diff?verify=true`
  - Temporary branches (`verify-<timestamp>-<hash>`)
  - Isolated verification namespace
  - Auto-cleanup
- **Webhooks**: `POST /v1/apps/:app/webhooks`
  - Real-time events (`build.*`, `deploy.*`)
  - JSON payloads with metadata
  - Retry + auth (Bearer/HMAC)
- **LLM Integration**: Monitor via webhooks, fix via verification branches

## 🌍 Environment Variables (planned)
- **Management**: `POST/GET/PUT/DELETE /v1/apps/:app/env`
- **Build-time**: Available during image creation
- **Runtime**: Injected into deployment environment
- **Security**: Sensitive values encrypted at rest
- **CLI**: `ploy env set/get/list/delete` commands

⸻

## 🔮 Next Steps
- Advanced readiness via Consul/Nomad health
- Per-app Unikraft recipes
- Keyless OIDC Cosign integration
- E2E testing with Nomad cluster
- Observability (Loki/Prometheus/Grafana)
- Traffic shifting (blue/green, canary)