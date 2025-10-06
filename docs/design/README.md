# Design Documents

Status checkboxes mirror the corresponding entries under `docs/tasks/shift/`.

- [x] [overview/README.md](overview/README.md) — Blueprint for the CLI-first
      architecture and Grid hand-off model (evergreen reference).
- [x] [event-contracts/README.md](event-contracts/README.md) — JetStream subject
      map and schema definitions powering the workflow event contract (Roadmap
      01; updated 2025-09-29 for discovery-driven client selection).
- [x] [checkpoint-metadata/README.md](checkpoint-metadata/README.md) — Enriches
      workflow checkpoints with stage metadata and artifact manifests for Grid
      consumers (Roadmap 17; updated 2025-09-29 to reflect discovery-only
      publishing).
- [x] [ipfs-artifacts/README.md](ipfs-artifacts/README.md) — Adds the IPFS
      gateway publisher for snapshot artifacts with JetStream metadata hooks
      (Roadmap 15; updated 2025-09-29 to note discovery-managed gateways).
- [x] [snapshot-metadata/README.md](snapshot-metadata/README.md) — Streams
      snapshot capture metadata to JetStream alongside gateway uploads (Roadmap
      16; updated 2025-09-29 for discovery-based routing).
- [x] [stage-artifacts/README.md](stage-artifacts/README.md) — Mirrors stage
      artifact envelopes onto JetStream for cache hydrators (Roadmap 18; updated
      2025-09-29 for discovery-driven JetStream selection).
- [ ] [mods/README.md](mods/README.md) — Parallel Mods workflow planner covering
      orw/LLM/human stages (task set shifting to `docs/tasks/mods/`); planner
      skeleton and knowledge base feedback landed 2025-09-27, but Grid job
      specs and healing feedback are pending `shift-mods-grid` restoration.
- [x] [knowledge-base/README.md](knowledge-base/README.md) — Fuzzy error
      classification feeding Mods/LLM remediation (Roadmap 20); classifier
      foundation, CLI ingest, and CLI evaluation slices completed 2025-09-27.
- [x] [integration-manifests/README.md](integration-manifests/README.md) — V2
      manifest schema, deterministic compiler, and CLI rewrite tooling landed
      2025-09-29.
- [ ] [shift/README.md](shift/README.md) — SHIFT roadmap overview aligning
      workstation-first slices; reopen pending Mods/Grid parity captured in
      `docs/tasks/shift/21-build-gate-reboot.md` follow-ups (updated 2025-10-07 to
      track Grid catalog registration).
- [x] [build-gate/README.md](build-gate/README.md) — Grid-aligned build gate
      with static checks, clarified sandbox scope, log streaming via Workflow
      RPC helper, and CLI summaries (Roadmap 21); stage planning/metadata landed
      2025-09-27, sandbox runner shipped 2025-10-05, the static check registry
      landed 2025-10-05, the Go vet adapter shipped 2025-09-27, log
      retrieval/log parsing shipped 2025-09-27, the build gate runner
      orchestration landed 2025-09-27, and CLI summaries with knowledge base
      guidance shipped 2025-10-07.
- [x] [build-gate/error-prone/README.md](build-gate/error-prone/README.md) —
      Java Error Prone adapter extends build gate multi-language coverage with
      registry wiring, CLI summaries, and `go test ./...` verification on
      2025-09-29 (Roadmap 21).
- [x] [build-gate/eslint/README.md](build-gate/eslint/README.md) — ESLint
      adapter extends static check coverage with manifest options (targets,
      config, rule overrides), CLI summary fixtures, and verification on
      2025-09-29 (Roadmap 21).
- [x] [discovery-alignment/README.md](discovery-alignment/README.md) — Aligns
      the CLI discovery client with Grid's expanded cluster-info payload
      (JetStream routes, feature gates, API endpoint, version) so workstation
      workflows honour feature availability (Roadmap TBD).
- [x] [workflow-rpc-alignment/README.md](workflow-rpc-alignment/README.md) —
      Aligns Ploy with Grid Workflow RPC/helper, locking the job spec schema
      (`image`, `command`, `env`, `resources`), subject alignment, SDK
      cancellation flows, archive surfacing, and CLI state-dir defaults (Roadmap
      22); latest update 2025-10-05.
- [x] [mods-grid-restoration/README.md](mods-grid-restoration/README.md) —
      Steps 1–4 landed 2025-10-05: repo materialisation, Mods lanes/job specs,
      and build-gate healing retries now ship via `ploy mod run`. Updated
      2025-10-07 to call out the dedicated Mods catalog repository + Grid
      registration follow-up (`docs/tasks/mods-grid/05-refactor.md`).
