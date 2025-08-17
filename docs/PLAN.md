# PLAN.md — Instructions

Changes implemented:
- **Lane builds**: A/B (Unikraft), C (OSv Java), D (Jail), E (OCI+Kontain), F (VM).
- **Supply chain**: CI produces SBOM (Syft), scans (Grype), signs (Cosign); controller **OPA** check before deploy.
- **Preview**: `https://<sha>.<app>.ployd.app` triggers build; naive readiness proxy.
- **CLI**: `apps new`, `.gitignore`-aware `push`, `open`.
- **Storage**: S3 abstraction (MinIO) + automatic artifact uploads.

Next steps to implement:
1. Replace naive readiness with Nomad API polling of alloc health, then proxy.
2. Integrate cosign keyless OIDC flow and key management.
3. Generate SBOM/signature in builders too (not only CI); upload both to storage.
4. Fill Unikraft per-app recipes and POSIX shim for lane B.
5. Enrich Nomad templates with Vault/Consul/env/volumes and canary rollout.

Critical gaps identified (Aug 2025 analysis):
6. Fix lane picker: Add Jib detection for Java/Scala Lane E vs C selection.
7. Implement Unikraft Lane B SSH support: Dropbear library, ssh.enabled flag, key injection.
8. Complete missing CLI commands: domains add, certs issue, debug shell, rollback.
9. Add TTL cleanup for preview allocations to prevent resource accumulation.
10. Implement image size caps per lane in OPA policies.
11. Fix Python C-extension detection in lane picker (should force Lane C).
12. Upload SBOM/signatures to storage after generation in builders.

Self-healing loop features (Aug 2025 addition):
13. Diff push with verification: `POST /v1/apps/:app/diff?verify=true` API and `ploy push --verify --diff` CLI to push diffs that create temporary git branches for isolated testing.
14. Webhook system: `POST /v1/apps/:app/webhooks` API to configure per-app webhooks for build/deploy events, enabling external LLM agents to monitor and react to deployment status.
