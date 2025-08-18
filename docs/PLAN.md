# PLAN.md — Instructions

Changes implemented:
- **Lane builds**: A/B (Unikraft), C (OSv Java), D (Jail), E (OCI+Kontain), F (VM).
- **Supply chain**: CI produces SBOM (Syft), scans (Grype), signs (Cosign); controller **OPA** check before deploy.
- **Preview**: `https://<sha>.<app>.ployd.app` triggers build; naive readiness proxy.
- **CLI**: `apps new`, `.gitignore`-aware `push`, `open`.
- **Storage**: S3 abstraction (MinIO) + automatic artifact uploads.

Next steps to implement:

**Phase 1: Critical Missing Basic Functionality**
1. ✅ **COMPLETED (2025-08-18)** Complete missing CLI commands: domains add, certs issue, debug shell, rollback.
2. ✅ **COMPLETED (2025-08-18)** Fix lane picker: Add Jib detection for Java/Scala Lane E vs C selection.
3. Fix Python C-extension detection in lane picker (should force Lane C).
4. App environment variables: `POST/GET/PUT/DELETE /v1/apps/:app/env` API and `ploy env` CLI commands to manage per-app environment variables that are available during build and deploy phases.
5. Replace naive readiness with Nomad API polling of alloc health, then proxy.

**Phase 2: Security & Supply Chain Hardening**
6. Integrate cosign keyless OIDC flow and key management.
7. Generate SBOM/signature in builders too (not only CI); upload both to storage.
8. Upload SBOM/signatures to storage after generation in builders.
9. Implement image size caps per lane in OPA policies.

**Phase 3: Platform Enhancement Features**
10. Implement Unikraft Lane B SSH support: Dropbear library, ssh.enabled flag, key injection.
11. Add TTL cleanup for preview allocations to prevent resource accumulation.
12. Enrich Nomad templates with Vault/Consul/env/volumes and canary rollout.

**Phase 4: Advanced Self-Healing & Automation**
13. Diff push with verification: `POST /v1/apps/:app/diff?verify=true` API and `ploy push --verify --diff` CLI to push diffs that create temporary git branches for isolated testing.
14. Webhook system: `POST /v1/apps/:app/webhooks` API to configure per-app webhooks for build/deploy events, enabling external LLM agents to monitor and react to deployment status.
15. Fill Unikraft per-app recipes and POSIX shim for lane B.
