# Folders

- `cmd/ploy/` — CLI (Bubble Tea, no GUI).
- `controller/` — Fiber-based REST API, builders, Nomad submitter, OPA gate, storage client.
- `build/` — scripts for OSv, Unikraft, OCI, Jail, VM images.
- `platform/nomad/templates/` — Nomad job templates per lane.
- `tools/lane-pick/` — lane auto-detector (heuristic).
- `configs/` — storage config.
- `docs/` — documentation (CLI, REST, LLM, STORAGE, etc.).
- `apps/` — your app sources (scaffolded via `ploy apps new`).

- `apps/*` — examples: go-helloweb, node-helloweb, python-fastapi, dotnet-webapi, scala-akka, java-ordersvc.
