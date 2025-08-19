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
- **Enhanced Lane Detection** (Aug 2025):
  - **Jib Plugin Detection**: Java/Scala projects with Jib → Lane E (containerless builds)
  - **Build System Support**: Gradle, Maven, SBT with comprehensive plugin detection
  - **Language Accuracy**: Proper Scala vs Java identification in mixed projects
  - **Python C-Extension Detection**: Multi-layered detection for C-extensions → Lane C
    - Source file detection: `.c`, `.cc`, `.cpp`, `.cxx`, `.pyx`, `.pxd` files
    - Library dependencies: numpy, scipy, pandas, psycopg2, lxml, pillow, cryptography
    - Build configuration: `ext_modules`, `Extension()`, `build_ext`, CMake integration
    - Cython support: Import detection and `.pyx` file analysis

⸻

## 🚀 Deployment
- Nomad templates per lane in `platform/nomad/`
- Jobs include health checks, Vault integration, canary rollouts, Consul registration
- Controller handles rendering, submission, health polling

⸻

## 🌐 Routing & Preview
- **Preview System**: `https://<sha>.<app>.ployd.app` triggers builds
  - **Nomad Health Monitoring**: Proper allocation health polling before routing ✅
  - **Smart Readiness**: Replaces naive HTTP checks with Nomad API integration ✅
  - **Error Handling**: Meaningful feedback for failed/pending deployments ✅
  - **Dynamic Discovery**: Endpoint detection based on allocation IP/port mapping ✅
- TTL cleanup for previews (planned)
- Domains: `manifests/<app>.yaml` configuration
- TLS: Certbot integration (planned), BYOC supported

⸻

## 👩‍💻 CLI (Go + Bubble Tea)
- `ploy apps new` – scaffold with /healthz
- `ploy push` – tar + stream to controller
- `ploy push --verify --diff` – verification branch testing (planned)
- `ploy open` – browser launch
- `ploy env` – manage app environment variables ✅
- `ploy domains/certs/rollback` – operations ✅
- **`ploy debug shell` – SSH-enabled debug instances** ✅
  - **Debug Build System**: Lane-specific debug builds with SSH daemon
  - **SSH Key Management**: Automatic RSA key pair generation per session
  - **Debug Isolation**: Nomad debug namespace with 2-hour auto-cleanup
  - **All Lane Support**: Unikraft, OCI, OSv, and jail debug environments
  - **Development Tools**: Pre-installed debuggers, profilers, and network tools
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

## 🌍 Environment Variables
- **Management**: `POST/GET/PUT/DELETE /v1/apps/:app/env` ✅
- **Build-time**: Available during image creation ✅
- **Runtime**: Injected into deployment environment ✅
- **Storage**: File-based persistence with JSON format
- **CLI**: `ploy env set/get/list/delete` commands ✅
- **Integration**: All lanes support environment variables in build and deploy phases

⸻

## 🔮 Next Steps
- Per-app Unikraft recipes
- Keyless OIDC Cosign integration
- E2E testing with Nomad cluster
- Observability (Loki/Prometheus/Grafana)
- Traffic shifting (blue/green, canary)