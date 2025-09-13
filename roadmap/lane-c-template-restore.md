# Lane C Template Restoration Plan

Goal: Restore richer service registrations and checks in Lane C templates (Java/OSv) now that HCL conversion and basic build path succeed.

Phased plan:

1) Health checks
- Reintroduce a single HTTP readiness/health check under the main service block.
- Ensure unique names (or rely on defaults without duplication). Validate with Nomad `-output` locally.

2) Service tags
- Add non-routing tags first (lane, version, runtime), then iterate to add routing labels.
- Verify that labels don’t introduce parse errors or conflicting constraints.

3) Metrics + JMX services
- Add metrics (HTTP) and JMX (TCP) as separate service blocks.
- Validate HCL → JSON conversion, then job submission on VPS test cluster.

4) Connect sidecar (optional)
- Wrap connect { sidecar_service {} } with CONNECT_ENABLED and ensure group-level constraints are satisfied.
- Only enable by default on clusters with Consul Connect configured.

5) Vault blocks (optional)
- Wrap vault {} in VAULT_ENABLED with correct policies present.
- Validate on clusters with Vault integration enabled.

6) Volumes and mounts
- Reintroduce host volumes only if required; otherwise keep minimized for dev.
- When enabling, ensure client node config provides host volumes with appropriate capabilities.

Validation checklist per phase:
- Run controller build POST and confirm 200.
- `nomad job run -output` via the job manager wrapper path to validate HCL.
- Confirm job start + basic health.

Rollback:
- Keep current minimal embedded templates as a safe baseline; re-enable features behind flags to avoid regressions.

