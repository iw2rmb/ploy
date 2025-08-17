# Ploy features

## 🎯 Core Purpose
	- Deliver maximum performance & smallest footprint PaaS by leveraging unikernels, OSv, Unikraft, jails, Kontain, and VMs.
	- Provide a Heroku-like developer experience (ploy push, ploy apps new, ploy open) while running apps in the most efficient lane possible.

⸻

## 🛠 Build Lanes (A–F)

Each source tree is auto-classified into a lane:
- Lane A – Unikraft Minimal (Go, C apps)
  - Ultra-small unikernel images via KraftKit.
  - Deterministic <app>-<sha>.img.
  - SBOM + signature generated during build.
- Lane B – Unikraft POSIX (Node, POSIX apps)
  - Musl libc + POSIX layer.
  - Supports POSIX syscalls for Node/Python-like apps.
  - SSH support with Dropbear (planned) for debug variants.
- Lane C – OSv Java
  - Jib (Gradle/Maven) → Tar → Capstan → OSv image.
  - Deterministic <app>-<sha>.qcow2.
  - Supports Java & Scala apps with custom MainClass.
- Lane D – FreeBSD Jails
  - Packs rootfs into <app>-<sha>-jail.tar.
  - Lightweight FreeBSD isolation, ideal for legacy POSIX apps.
- Lane E – OCI + Kontain
  - Docker/Jib build to harbor.local/ploy/<app>:<sha>.
  - Runs under Docker runtime io.kontain for better performance & isolation.
- Lane F – VM (Packer)
  - Builds VM images (<app>-<sha>.img) for workloads requiring full OS.
  - Provides maximum compatibility fallback.

⸻

## ⚙️ Builders
- Scripts per lane in build/:
  - build/osv/java/…, build/kraft/…, build/oci/…, build/jail/…, build/packer/….
- SBOM (Syft) + Signatures (Cosign) automatically produced when tools are installed.
- Deterministic image naming (<app>-<sha>).
- Can run standalone or invoked via controller.

⸻

## 📦 Supply Chain Security
- SBOMs: Generated during builds and in CI with Syft.
- Vulnerability scans: With Grype (CI).
- Signing: With Cosign (key-based or keyless OIDC).
- Storage upload: SBOM/signatures uploaded to object storage after generation (planned).
- OPA Policy Enforcement:
  - Requires signature + SBOM before deploy.
  - Blocks SSH in prod unless break-glass flag is set.
  - Image size caps per lane (planned).
  - Enhanced lane detection (Jib plugin detection, Python C-extensions).

⸻

## 🚀 Deployment (Nomad-based)
- Nomad templates per lane in platform/nomad/templates/.
- Each job includes:
  - Health checks.
  - Vault integration (vault { policies = ["default"] }).
  - Canary rollout strategy (update { canary=1, auto_promote, auto_revert }).
  - Consul service registrations.
- controller/nomad/ handles rendering and submission.
- controller/nomad/client.go polls allocations for health readiness.

⸻

## 🌐 Routing & Preview
- Preview routes:
https://<sha>.<app>.ployd.app → triggers build pipeline and proxies when ready.
- TTL cleanup for preview allocations (planned) to prevent resource accumulation.
- Domain management:
  - Per-app manifests/<app>.yaml defines custom domains.
  - ploy open resolves and opens correct domain.
- TLS:
  - Certbot integration planned (auto-issue).
  - Bring-your-own cert supported.

⸻

## 👩‍💻 Developer Experience
- CLI (ploy) in Go + Bubble Tea TUI.
  - ploy apps new --lang <go|node> --name <app> → scaffolds app with /healthz.
  - ploy push → tars repo (respects .gitignore), streams to controller, triggers lane build.
  - ploy push --verify --diff <file> → pushes diff to verification branch for isolated testing (planned).
  - ploy open <app> → opens deployed app in browser.
  - ploy domains add <app> <domain> → updates Consul and ingress (planned).
  - ploy certs issue <domain> → obtains cert via ACME HTTP-01 (planned).
  - ploy debug shell <app> → builds debug variant with SSH and prints command (planned).
  - ploy rollback <app> <sha> → restores previous release (planned).
- Heroku-like workflow: push → build → deploy → open.
- Self-healing loop support for external LLM agents via diff push and webhooks.

⸻

## 🗄 Storage Layer
	- Abstracted S3 client with MinIO as default.
	- Config: configs/storage-config.yaml.
	- Controller uploads artifacts (image, sbom, signature) → artifacts/<app>/<sha>/.
	- Easy migration to other S3 backends (Ceph, AWS S3, etc).

⸻

## 🔬 Example Apps
- Go: apps/go-helloweb/
- Node: apps/node-helloweb/
- Python: apps/python-fastapi/
- .NET: apps/dotnet-webapi/
- Scala: apps/scala-akka/
- Java: apps/java-ordersvc/

All include /healthz endpoint on port 8080.

⸻

## 🧪 CI/CD Integration
- GitHub Actions:
  - Build controller, SBOM, scan, sign (keyless).
- GitLab CI:
  - Validate (lane pick), build, supply-chain checks, deploy.
- Output artifacts (repo.sbom.json, signatures) uploaded for traceability.

⸻

## 🤖 Self-Healing Loop Integration
- **Diff Push with Verification** (planned):
  - `POST /v1/apps/:app/diff?verify=true` API endpoint.
  - Creates temporary git branches (`verify-<timestamp>-<hash>`) for safe testing.
  - Isolated verification deployments in separate Nomad namespace.
  - Automatic cleanup of verification branches and deployments.
  - CLI: `ploy push --verify --diff <file>` for LLM agent integration.

- **Webhook System** (planned):
  - Per-app webhook configuration via `POST /v1/apps/:app/webhooks`.
  - Real-time build/deploy event streaming (`build.started`, `build.completed`, `build.failed`, etc.).
  - Structured JSON payloads with timestamps, log levels, and metadata.
  - Webhook retry logic with exponential backoff.
  - Authentication support (Bearer tokens, HMAC signatures).

- **LLM Agent Integration**:
  - External agents can monitor deployments via webhooks.
  - Push fixes safely via verification branches.
  - Implement automated self-healing loops with proper isolation.

⸻

## 🔮 Extensibility / Next Steps
- Advanced readiness: switch preview to Consul/Nomad service health resolution.
- Richer lane recipes (Unikraft with Node/Python per-app configurations).
- Keyless OIDC flow integration for Cosign.
- Automated end-to-end tests with Nomad cluster in CI.
- Observability pipeline (Loki/Prometheus/Grafana).
- Traffic shifting (blue/green, weighted canary).