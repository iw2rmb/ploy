# Mods E2E Flow (Spring Boot Upgrade Example)

Use this reference when reasoning about Mods end-to-end runs across Ploy and the
shared lane catalog. The example assumes a Java repository where we want to bump
the Spring Boot version via an OpenRewrite recipe.

## Roles At A Glance

- **Ploy** orchestrates workstation flows: claims tickets, materialises the
  repo, invokes the Mods planner, applies healing logic, and renders summaries.
- **Lane Catalog** is the shared catalogue of lane definitions, cache-key rules,
  and job metadata. It provides canonical TOML specs (e.g. `mods-plan.toml`) and
  helper utilities for composing jobs.
- The control plane executes the resulting stages. It consumes job payloads constructed
  from catalog lanes and reports checkpoints/artefacts back to Ploy.

## Step-By-Step Execution

1. **Ticket Claim & Manifest Compile**  
   - Ploy runs `ploy mod run --repo-*` to claim a ticket and compile a manifest
     such as `smoke.toml`, which lists required lanes (`mods-plan`,
     `mods-java`, `mods-llm`, `mods-human`, `go-native`).  
   - The catalog supplies those lane definitions (or Ploy’s local copy until the
     migration) so cache keys and job specs are deterministic.

2. **Mods Plan Stage (`mods-plan`)**  
   - Ploy schedules the planner stage using the catalog `mods-plan` spec.  
   - The runtime runs the container defined in the lane, scanning the repo and
     knowledge base to produce planner metadata identifying the Spring Boot
     upgrade recipe and any follow-on stages.

3. **OpenRewrite Apply/Generate (`mods-java`)**  
   - Ploy enqueues `orw-apply` and `orw-gen` stages with lane `mods-java`.  
   - The runtime executes OpenRewrite against the repo, generating diffs that bump the
     Spring Boot dependency. Artefacts (diff bundles, logs) flow back via
     checkpoints.

4. **LLM Execution (`mods-llm`)**  
   - If the planner requested additional adjustments, Ploy schedules the
     `mods-llm` lane. The catalog definition may include GPU accelerators.  
   - The runtime runs the language-model job, applying follow-up fixes.

5. **Human Gate (`mods-human`)**  
   - When a manual check is required, Ploy adds the `mods-human` stage.  
   - The orchestrator pauses the workflow until a reviewer approves or edits the change.

6. **Build/Test Verification (`go-native`)**  
   - Standard build-gate, static checks, and tests run using their respective
     lanes. Successful completion means the Spring Boot upgrade passed.

7. **Healing Loop (Optional)**  
   - If the build gate fails (e.g. missing dependency), Ploy collects failure
     metadata and re-invokes the planner with healing suffixes (stages named
     `#heal1`, `#heal2`, etc.).  
   - The catalog lanes are reused for these healing stages. The runtime executes them and
     reruns build/test until success or retry limits are hit.

8. **Outcome**  
   - Ploy summarises planner recommendations, applied recipes, healing attempts,
     and final checkpoints.  
   - The control plane stores run history and artefacts for downstream tooling.  
   - The lane catalog continues to act as the authoritative source for lane specs used in
     future runs.

## Migration Notes

- Mods lanes live in `ploy/lanes/*.toml` (mirrored into the catalog) and follow
  the catalog schema. Migrating them into the shared registry removes duplication and lets
  the orchestrator consume the same definitions without relying on Ploy.
- Live smoke tests remain blocked until the Mods lane pack and manifest
  are published through the shared catalog; the workstation harness (`go test -tags e2e`) uses
  the in-memory stub meanwhile.
