# SBOM Hooks Roadmap

Common ground:
- Add deterministic `sbom` and `hook` jobs around gate flow.
- Execute SBOM before each gate stage (`pre_gate`, `post_gate`, `re_gate`).
- Load hooks from root-level `hooks` entries in mig spec.
- Execute hook steps by stack + SBOM diff conditions.
- Support `once` with hash-based idempotency.

Phases:
- [ ] `phase-1-contracts-and-scheduling.yaml`
- [ ] `phase-2-hook-runtime-and-persistence.yaml`
- [ ] `phase-3-sbom-images-and-docker-labels.yaml`
