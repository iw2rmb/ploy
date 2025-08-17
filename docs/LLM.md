# LLM.md — v6 Plan

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
