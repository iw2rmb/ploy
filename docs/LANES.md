# Ploy Lanes A–G

This document describes Ploy’s deployment lanes, how detection works, what each lane builds and deploys, platform dependencies, and best practices for application authors and platform operators.

## Overview

Ploy classifies apps into lanes A–G based on repository structure and explicit overrides. Each lane maps to a different isolation/runtime model and build pipeline:

- Lane A — Unikraft Minimal (Go, C)
- Lane B — Unikraft POSIX (Node.js, Python)
- Lane C — OSv JVM (Java/Scala/Kotlin)
- Lane D — FreeBSD Jails (rootfs tar)
- Lane E — Containers (OCI, Kontain optional)
- Lane F — Full VMs (qcow2)
- Lane G — WebAssembly (WASI runtimes)

Lane selection can be explicit via `lane` query param or environment override. Otherwise, the lane picker infers lane based on build files and language markers.

## Lane Details

### Lane A — Unikraft Minimal
- Runtime: Unikraft unikernels for minimal libc apps (Go via tinygo or C)
- Build: Unikraft/KraftKit pipeline, produces `.img`
- Deploy: Nomad (template: `platform/nomad/lane-a-unikraft.hcl`)
- Health: HTTP on `:8080/healthz` (application responsibility)
- Notes: smallest footprint, fastest boot; limited POSIX surface

### Lane B — Unikraft POSIX
- Runtime: Unikraft POSIX layer for Node/Python
- Build: Unikraft with language-specific glue; produces `.img`
- Deploy: Nomad (template: `platform/nomad/lane-b-unikraft-posix.hcl`)
- Health: HTTP `:8080/healthz`
- Notes: richer POSIX environment than A; slightly larger images

### Lane C — OSv JVM
- Runtime: OSv micro-VM for JVM applications
- Build: Jib → OSv image tooling; produces `.img/.qcow2`
- Deploy: Nomad (template: `platform/nomad/lane-c-java.hcl`)
- Health: HTTP `:8080/healthz`
- Notes: excellent fit for JVM services; fast boot; adjustable `MainClass`

### Lane D — FreeBSD Jails
- Runtime: FreeBSD jail with rootfs tar
- Build: Assembles jail rootfs; produces `<app>-<sha>-jail.tar`
- Deploy: Nomad (template: `platform/nomad/lane-d-jail.hcl`)
- Health: HTTP `:8080/healthz`
- Notes: pragmatic lane for legacy POSIX apps

### Lane E — Containers (OCI)
- Runtime: Docker/OCI; Kontain runtime optional when available
- Build (Dev VPS): Kaniko builder job executes container build (no Docker in API)
  - Flow: API uploads source tar, renders `lane-e-kaniko-builder.hcl`, submits batch job, waits terminal, verifies registry manifest, then deploys app (`lane-e-oci-kontain.hcl`).
  - Registry: `registry.dev.ployman.app` (Dev) without auth. Tag: `<registry>/<app>:<sha>`
- Health: HTTP `:8080/healthz`; Traefik routes `Host(<app>.<domain>)`
- Notes: easiest onramp via Dockerfile. Supports autogeneration (opt-in) for trivial Go/Node apps.

#### Dockerfile Autogeneration
- Default: If `Dockerfile` is missing, API returns 400 instructing to add one.
- Opt-in: `autogen_dockerfile=true` query param (or `PLOY_AUTOGEN_DOCKERFILE=true`) generates a minimal Dockerfile using centralized detection:
  - Go: multi-stage build → distroless
  - Node: Node 20 alpine → run `index.js`
  - JVM (Gradle/Maven):
    - Build: Gradle `gradle:8-jdk<major>` or Maven `maven:3-eclipse-temurin-<major>`
    - Runtime: `eclipse-temurin:<major>-jre`
    - Entrypoint: main class if detected, else `java -jar /app/app.jar`
  - .NET: `mcr.microsoft.com/dotnet/sdk:<ver>` → `mcr.microsoft.com/dotnet/aspnet:<ver>`, entrypoint `dotnet <Project>.dll`
  - Python: `python:<ver>-slim`; if app server present in deps, prefer `gunicorn` or `uvicorn`; otherwise `python app.py`
- Best Practice: Keep an explicit Dockerfile in the repo for clarity and control. Autogen is for bootstrap/demos only.

### Lane F — Full VMs
- Runtime: full virtualization (qcow2)
- Build: image assembly; produces `vm.img/qcow2`
- Deploy: Nomad (template: `platform/nomad/lane-f-vm.hcl`)
- Health: HTTP `:8080/healthz`
- Notes: use when OS-level control or compatibility dictates

### Lane G — WebAssembly
- Runtime: WASI (e.g., wazero), sandboxed execution
- Build: produces `*.wasm`
- Deploy: Nomad with WASM runtime template
- Health: HTTP `:8080/healthz` (via host shim)
- Notes: Great for small services; multi-language; extremely fast start

## Lane Detection

The lane picker examines project markers:
- Language/build files (go.mod, package.json, pom.xml/gradle, etc.)
- Jib plugins, OSv/Unikraft configs
- Explicit overrides via query `lane`, env `LANE_OVERRIDE`, or heuristics

If detection fails, the system defaults to Lane E in Dev to maximize success, but enforcement/policies still apply.

## Deployment Pipeline Summary

1) Client push (async by default) uploads source tar to API.
2) API unpacks source, detects lane, and prepares artifacts.
3) Lane-specific build step runs:
- A/B: Unikraft image build
- C: OSv image build
- D: Jail rootfs assembly
- E: Kaniko builder job (Dev) builds/pushes container image
- F: VM image assembly
- G: WASM build
4) Policy checks (SBOM/signing/size caps) and metadata uploads to storage.
5) Template render + Nomad submit; wait until healthy; Traefik routes HTTPS.

## Testing & Observability

- E2E scripts: `tests/lanes/test-lane-deploy.sh` and Go tests under `tests/e2e`.
- Logs: `GET /v1/apps/:app/logs`, plus platform logs (Traefik) via `/v1/platform/traefik/logs`.
- Preview URLs: `https://<sha>.<app>.<domain>/<HEALTH_PATH>` with fallback to `https://<app>.<domain>`.

## Environment & Config

- Controller: `PLOY_CONTROLLER=https://api.dev.ployman.app/v1`
- Registry (Dev): `registry.dev.ployman.app` (no auth)
- Storage: SeaweedFS Filer (Dev): `http://seaweedfs-filer.service.consul:8888`
- Common query params:
  - `lane=A|B|...` force lane
  - `autogen_dockerfile=true` enable Dockerfile autogeneration (Lane E only)
  - `async=true` use async building (default via CLI)

## Best Practices

- Prefer explicit Dockerfiles and lane-appropriate project structure.
- Ensure `/healthz` endpoint and `PORT=8080` binding.
- Keep images small; SBOMs and signatures are strongly encouraged.
- Use Dev registry for platform images and app builds; avoid external registries on the VPS path.

## Limitations (Dev VPS)

- API runs as a Nomad job without Docker — container builds use Kaniko.
- Traefik runs as a Nomad job; logs available via the Dev API.
- Some advanced lane features (e.g., mesh, Vault) are disabled in Dev templates.
