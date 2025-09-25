# Ploy Shift Plan

## Purpose
Reset Ploy as a stateless workflow brain that runs on demand, hands all job execution to Grid, and keeps the mods toolchain blazing fast for every language. Ploy stops behaving like a long-lived API with Nomad dependencies and instead becomes a CLI/agent that reads events from JetStream, decides what to run next, and exits.

## Ploy + Grid Operating Model
- Grid owns the control surface (webhooks, scheduling, node pools, cache promotion, network policy enforcement) and persists hot signals in JetStream plus cold artifacts in IPFS.
- Ploy consumes those events, evaluates mods/workflow DAGs, and submits follow-up jobs back to Grid. Every run checkpoint, artifact pointer, and decision lives in JetStream streams or IPFS so retries are stateless.
- Mods jobs (orw-apply/orw-gen/human-in-the-loop/llm-plan/llm-exec/…) remain first-class workflows: Ploy assembles the job graph, Grid executes the jobs, and the build gate still performs SBOM, vuln, and static-analysis checks before promotion.

## Drop Immediately
- All Nomad/Consul/Traefik templates, wrappers, and deployment logic (`internal/orchestration`, embedded HCL, Ansible Nomad playbooks, Consul KV usage).
- Long-running API/service assumptions (platform vs. app routing, system jobs, controller ingress). Replace with CLI-only interfaces and JetStream consumers.
- SeaweedFS-specific artifact plumbing where IPFS (cold) or JetStream (hot) can satisfy retention requirements.
- Legacy lane metadata tied to Nomad job specs; keep only what is required to describe build/test runtimes for Grid.

## Build/Refactor First
1. **Event Contract** – Define JetStream subjects and IPFS paths that describe workflow inputs, checkpoints, job statuses, database snapshots, and cached build layers.
2. **Workflow Runner CLI** – Ship a `ploy workflow run` entrypoint that pulls a ticket from JetStream, reconstructs the DAG (mods, build gate, tests), pushes job specs to Grid, and exits.
3. **Lane Redesign** – Encode lanes as build profiles targeting the fastest boot/runtime per language (e.g., WASM for Node/C++, GraalVM incremental for Java). Capture cache keys so Grid can reuse artifacts automatically.
4. **Snapshot Toolkit** – Integrate database snapshot rules (source connection, strip/mask directives, synthetic data generators) and push diff outputs to JetStream/IPFS so subsequent test runs replay quickly.
5. **Integration Manifests** – Define the TOML/Markdown schema that test authors use to declare topology (allowed service flows), required fixtures, and required lanes. Ploy parses and passes the constraints to Grid’s topology compiler.
6. **Commit-Scoped Environments** – Provide commands to materialize `<sha>-<app>` builds on demand, referencing the corresponding lane caches, DB snapshots, and integration manifests.
7. **Aster Hook** – Align with the upcoming `Aster` pipeline by describing how AST-pruned code is produced, handed to Grid builds, and cached (include toggle per workflow step).

## Data & Storage Expectations
- JetStream remains the hot path for events, logs, run metadata, and cache-coordination messages.
- IPFS (or compatible object storage) keeps long-lived artifacts: build outputs, database snapshot archives, diff reports, and audit logs for mods workflows.
- Persistent workspace metadata (hash IDs, eviction policies, ownership) is modeled so Grid can claim/release caches without per-team scripting.

## Testing & Tooling Priorities
- Rework test harnesses to run entirely against the new JetStream/Grid stubs (no Nomad fakes). Provide unit coverage for DAG reconstruction, cache key resolution, DB snapshot diffing, and Aster integration hooks.
- Refresh documentation (`README.md`, `docs/LANES.md`, `docs/TESTING.md`, mods guides) to reflect the Grid-centric architecture and remove Nomad references.

## Success Criteria
- Mods workflows execute end-to-end through Grid with faster build/test cycles than today’s Nomad-based runs.
- Developers can request any commit/lane combination (`<sha>-<app>`) with deterministic caches, DB snapshots, and topology guardrails applied automatically.
- No permanent services are required: if Ploy CLI is idle, Grid still ingests webhooks and queues work for the next invocation.
