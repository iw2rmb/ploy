# SBOM Hooks Remediation Roadmap

Common ground:
- Replace SBOM and hook placeholder runtime with real executable behavior.
- Add hook jobs to plan only when condition evaluation is true for that cycle.
- Fail run deterministically on spec/runtime/storage errors; no silent skip and no fake success.
- Make roadmap completion depend on end-to-end runtime evidence.

Phases:
- [ ] `phase-1-conditional-planning-and-preflight.yaml` <!-- evidence:phase-1-conditional-planning-and-preflight -->
- [ ] `phase-2-runtime-execution-and-ingestion.yaml` <!-- evidence:phase-2-runtime-execution-and-ingestion -->
- [ ] `phase-3-delivery-gates-and-observability.yaml` <!-- evidence:phase-3-delivery-gates-and-observability -->
