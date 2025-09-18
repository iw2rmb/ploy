# Ploy Test Scripts

Shell-based checks that complement the Go test suites. Scripts are grouped by workflow so you can validate the Docker-only controller quickly from a VPS session or local clone.

## Running Scripts

```bash
ssh ploy@$TARGET_HOST
cd ~/ploy/tests/scripts
./test-<name>.sh
```

All active scripts rely on `common/test-utils.sh` for logging helpers; sourcing it is optional for the lightweight lane-D checks introduced during the consolidation.

## Layout (Docker Lane Focus)

- **Lane D build + artifact checks**
  - `test-image-size-caps.sh` – Asserts the controller exposes only Docker lane limits and runs the focused Go test for `internal/build/resources.go`.
  - `test-size-caps-unit.sh` – Exercises `internal/utils/image_size.go` helpers and fails if any deprecated lane identifiers reappear.
  - `test-storage-fix-verification.sh`, `test-upload-helpers-unit.sh` – Validate artifact persistence and upload helpers that support the Docker lane builds.
- **Platform operations**
  - `test-app-destroy.sh`, `test-ttl-cleanup.sh`, `test-blue-green-deployment.sh` – Smoke-test app lifecycle behaviour via the controller API.
  - `test-opa-policy.sh`, `test-enhanced-policy-enforcement.sh` – Confirm OPA rules (including size caps) continue to pass for the Docker runtime.
- **Networking & certificates**
  - `test-dns-integration.sh`, `test-dns-propagation.sh`, `test-platform-wildcard-certificates.sh`, `test-dev-wildcard-certificate.sh`, `test-ssl-deployment.sh` – Cover CoreDNS-backed resolution and ACME/Traefik flows.
- **Environment & CLI**
  - `test-env-cli.sh`, `test-env-vars.sh`, `test-git-validation-unit.sh` – Sanity checks for CLI-driven deployments and repo validation now that builds are Docker-only.
- **Legacy / archived**
  - `archive/` retains scripts that referenced the retired lanes. Keep them for historical context; they should not be executed against the Docker-only controller.

## Conventions

- Scripts abort on first failure (`set -euo pipefail`).
- Use environment variables (e.g., `TARGET_HOST`, `PLOY_CONTROLLER`) rather than hard-coding endpoints.
- Prefer running targeted Go tests (via `go test -run ...`) inside scripts when checking controller helpers—this keeps the suite fast and aligned with the Go modules.

## Updating or Adding Scripts

1. Keep lane assumptions explicit; if a script expects Docker-only behaviour, mention it in the usage text.
2. Reuse helpers from `common/test-utils.sh` for colourised output and assertions.
3. Add new scripts alongside the relevant category above, and update this README with a one-line description.

The previous lane matrix (A–G) has been fully removed. New scripts should validate Docker runtime behaviour and avoid referencing deprecated lanes or Consul DNS endpoints.
