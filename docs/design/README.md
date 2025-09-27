# Design Documents

Status checkboxes mirror the corresponding entries under `roadmap/shift/`.

- [x] [overview/README.md](overview/README.md) — Blueprint for the CLI-first architecture and Grid hand-off model (evergreen reference).
- [x] [event-contracts/README.md](event-contracts/README.md) — JetStream subject map and schema definitions powering the workflow event contract (Roadmap 01).
- [x] [checkpoint-metadata/README.md](checkpoint-metadata/README.md) — Enriches workflow checkpoints with stage metadata and artifact manifests for Grid consumers (Roadmap 17).
- [x] [ipfs-artifacts/README.md](ipfs-artifacts/README.md) — Adds the IPFS gateway publisher for snapshot artifacts with JetStream metadata hooks (Roadmap 15).
- [x] [snapshot-metadata/README.md](snapshot-metadata/README.md) — Streams snapshot capture metadata to JetStream alongside gateway uploads (Roadmap 16).
- [x] [stage-artifacts/README.md](stage-artifacts/README.md) — Mirrors stage artifact envelopes onto JetStream for cache hydrators (Roadmap 18).
- [x] [mods/README.md](mods/README.md) — Parallel Mods workflow planner covering orw/LLM/human stages (Roadmap 19); runner parallel execution landed 2025-09-27 after planner, knowledge base, and CLI/Grid wiring milestones.
- [x] [knowledge-base/README.md](knowledge-base/README.md) — Fuzzy error classification feeding Mods/LLM remediation (Roadmap 20); classifier foundation, CLI ingest, and CLI evaluation slices completed 2025-09-27.
- [ ] [build-gate/README.md](build-gate/README.md) — Grid-aligned build gate with static checks, clarified sandbox scope, and log streaming via Workflow RPC helper (Roadmap 21); stage planning/metadata landed 2025-09-27 with sandbox/log work pending.
- [x] [workflow-rpc-alignment/README.md](workflow-rpc-alignment/README.md) — Aligns Ploy with Grid Workflow RPC/helper, locking the job spec schema (`image`, `command`, `env`, `resources`) and subject alignment (Roadmap 22); SDK wrapper, job spec composition, subject realignment, and helper adoption (auth + retries) landed by 2025-10-01.
