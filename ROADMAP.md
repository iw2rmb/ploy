# Type System Hardening (Boundary Layers)

Scope: Harden weakly typed boundaries without changing external behavior. Replace ad‑hoc `map[string]any` payloads with DTOs, type GitLab client I/O, add a generic transfer client, type event payloads, and add `StepManifest` option accessors. Keep APIs stable.

Documentation: docs/api/OpenAPI.yaml; internal/domain/types/*; internal/server/handlers/*; internal/nodeagent/gitlab/mr_client.go; internal/cli/transfer/client.go; internal/stream/hub.go; internal/workflow/contracts/step_manifest.go

Legend: [ ] todo, [x] done.

## Phase 1 — GitLab client + Transfer generics
- [x] Replace GitLab request/response maps with DTOs — Remove runtime asserts, ensure schema safety
  - Component: node agent
  - Change: internal/nodeagent/gitlab/mr_client.go — add `mrCreatePayload` and `mrCreateResponse{ WebURL string \`json:"web_url"\` }`; update `CreateMR` to encode/decode DTOs
  - Test: internal/nodeagent/gitlab/mr_client_test.go — `httptest.Server` fixtures (201, 429, 5xx); expect correct URL extraction and backoff behavior

- [ ] Introduce generic HTTP helper for transfer client — Compile‑time request/response typing
  - Component: CLI
  - Change: internal/cli/transfer/client.go — add `doReq[TReq any, TRes any]`; refactor `requestSlot`, `Commit`, `Abort` to use typed DTOs
  - Test: internal/cli/transfer/client_test.go — table tests against `httptest.Server`; expect slot decode, error paths preserved

## Phase 2 — Typed handler responses (DTOs)
- [ ] Diffs list endpoint uses DTOs — Stable JSON schema, no map literals
  - Component: server
  - Change: internal/server/handlers/handlers_diffs.go — introduce `diffItem`, `diffListResponse{ Diffs []diffItem }`; replace `map[string]any` encodes
  - Test: internal/server/handlers/handlers_diffs_test.go — handler unit tests; expect identical JSON fields/values

- [ ] Diff get endpoint uses DTOs — Typed response metadata
  - Component: server
  - Change: internal/server/handlers/handlers_diffs.go — add typed metadata struct used when `download!=true`
  - Test: internal/server/handlers/handlers_diffs_test.go — metadata case; expect stable keys and types

- [ ] Node logs create uses DTO — Typed ack payload
  - Component: server
  - Change: internal/server/handlers/nodes_logs.go — define `nodeLogCreateResponse{ ID int64, ChunkNo int32 }`; replace `map[string]interface{}`
  - Test: internal/server/handlers/nodes_logs_test.go — success path; expect created JSON matches schema

## Phase 3 — Event typing + Options helpers
- [ ] Type ticket event payload at hub boundary — Prevent accidental non‑JSON payloads
  - Component: server (events/stream)
  - Change: internal/stream/hub.go — change `PublishTicket(ctx, id string, ticket modsapi.TicketSummary)`; internal marshal stays generic; adjust call sites if needed
  - Test: internal/stream/hub_test.go — compile‑time usage; publish/subscribe round‑trip with typed payload

- [ ] Add `StepManifest` option accessors — Centralize `Options` map handling
  - Component: workflow contracts, node agent, CLI
  - Change: internal/workflow/contracts/manifest_options.go — add `OptionString`, `OptionBool` methods on `StepManifest`; use in `internal/nodeagent/execution.go` (MR flow) and `cmd/ploy/mod_run.go` where reading options
  - Test: internal/workflow/contracts/manifest_options_test.go — unit tests; adjust `execution` tests to use accessors; expect same behavior

