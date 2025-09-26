# Design Documents

Status checkboxes mirror the corresponding entries under `roadmap/shift/`.

- [x] [overview/README.md](overview/README.md) — Blueprint for the CLI-first architecture and Grid hand-off model (evergreen reference).
- [x] [event-contracts/README.md](event-contracts/README.md) — JetStream subject map and schema definitions powering the workflow event contract (Roadmap 01).
- [x] [checkpoint-metadata/README.md](checkpoint-metadata/README.md) — Enriches workflow checkpoints with stage metadata and artifact manifests for Grid consumers (Roadmap 17).
- [x] [ipfs-artifacts/README.md](ipfs-artifacts/README.md) — Adds the IPFS gateway publisher for snapshot artifacts with JetStream metadata hooks (Roadmap 15).
- [x] [snapshot-metadata/README.md](snapshot-metadata/README.md) — Streams snapshot capture metadata to JetStream alongside gateway uploads (Roadmap 16).
- [x] [stage-artifacts/README.md](stage-artifacts/README.md) — Mirrors stage artifact envelopes onto JetStream for cache hydrators (Roadmap 18).
- [ ] [mods/README.md](mods/README.md) — Parallel Mods workflow planner covering orw/LLM/human stages (Roadmap 19); planner skeleton and knowledge base metadata loop completed 2025-09-26.
- [ ] [knowledge-base/README.md](knowledge-base/README.md) — Fuzzy error classification feeding Mods/LLM remediation (Roadmap 20).
- [ ] [build-gate/README.md](build-gate/README.md) — Grid-aligned build gate with static checks and log parsing (Roadmap 21).
