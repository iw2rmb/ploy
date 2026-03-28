# Internal Naming Clarity Roadmap

Scope: Remove ambiguous, overloaded, and conflicting names in high-usage `internal/` surfaces by applying direct renames with no backward-compatibility aliases.

Documentation: `roadmap/rename.md`, `README.md`, `internal/server/README.md`, `internal/tui/README.md`, `internal/client/README.md`.

- [x] 1.1 Rename shared server handler helpers to explicit request/error names
  - Type: determined
  - Component: `internal/server/handlers/ingest_common.go`, `internal/server/handlers/*.go`, `internal/server/handlers/ingest_common_test.go`
  - Implementation:
    1. Rename `httpErr` to `writeHTTPError` and update all handler call sites.
    2. Rename `parseParam` to `parseRequiredPathID` and update all typed-ID path parsing call sites.
    3. Rename server `DecodeJSON` to `decodeRequestJSON` and update all handler call sites and tests.
    4. Delete old helper identifiers so only renamed helpers remain.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `rg -n '\b(httpErr|parseParam|DecodeJSON)\b' internal/server/handlers`.
  - Reasoning: high (10 CFP)

- [x] 1.2 Disambiguate CLI JSON decode helper from server request decode helper
  - Type: determined
  - Component: `internal/cli/httpx/httpx.go`, `internal/cli/**/*.go`
  - Implementation:
    1. Rename CLI `httpx.DecodeJSON` to `httpx.DecodeResponseJSON`.
    2. Update all CLI call sites to the renamed helper.
    3. Delete old helper identifier so CLI decode semantics have one explicit name.
  - Verification:
    1. Run `go test ./internal/cli/...`.
    2. Run `rg -n '\bDecodeJSON\b' internal/cli`.
  - Reasoning: medium (6 CFP)

- [x] 1.3 Rename server route registration API to avoid `net/http` naming collision
  - Type: determined
  - Component: `internal/server/http_server.go`, `internal/server/**/*.go`, `cmd/ployd/**/*.go`
  - Implementation:
    1. Rename `HTTPServer.Handle` to `RegisterRoute` and `HTTPServer.HandleFunc` to `RegisterRouteFunc`.
    2. Rename `HTTPServer.HandleFuncAllowQueryToken` to `RegisterRouteFuncAllowQueryToken`.
    3. Update all server bootstrap and route registration call sites to renamed methods.
    4. Delete old method names so route registration has one explicit API.
  - Verification:
    1. Run `go test ./internal/server/... ./cmd/ployd/...`.
    2. Run `rg -n '\bHandleFuncAllowQueryToken\b|\bHandleFunc\b|\bHandle\(' internal/server cmd/ployd`.
  - Reasoning: medium (8 CFP)

- [x] 1.4.1 Normalize `mod` type terminology to `mig` in workflow contracts
  - Type: determined
  - Component: `internal/workflow/contracts/migs_spec.go`, `internal/workflow/contracts/mod_image.go`, `internal/workflow/contracts/migs_spec_parse.go`, `internal/workflow/contracts/*_test.go`, `internal/workflow/**/*.go`
  - Implementation:
    1. Rename `ModsSpec`/`ModStep`/`ModStack` to `MigSpec`/`MigStep`/`MigStack` in workflow contract definitions.
    2. Rename contract parser/build helpers that encode `mod` naming to `mig` naming in `internal/workflow/contracts`.
    3. Update workflow package call sites and tests to compile and validate against renamed contract symbols.
    4. Remove old contract `mod` symbols so workflow contracts expose one `mig` vocabulary.
  - Verification:
    1. Run `go test ./internal/workflow/contracts ./internal/workflow/...`.
    2. Run `rg -n '\bModsSpec\b|\bModStep\b|\bModStack\b' internal/workflow`.
  - Reasoning: high (10 CFP)

- [x] 1.4.2 Normalize `mod` job metadata and orchestration symbols to `mig`
  - Type: assumption-bound
  - Component: `internal/workflow/contracts/job_meta.go`, `internal/nodeagent/**/*.go`, `internal/server/handlers/**/*.go`, `internal/cli/**/*.go`, `internal/workflow/**/*.go`
  - Assumptions: Renames are API-internal and Go-level only; wire schema keys and persisted JSON values remain unchanged where required by stored data contracts (for example `kind:"mig"` and existing JSON field tags).
  - Implementation:
    1. Rename `JobKindMod` to `JobKindMig` and rename `NewModJobMeta`-style constructors/helpers to `NewMigJobMeta`.
    2. Update nodeagent orchestration paths to use renamed job metadata symbols end-to-end.
    3. Update server handler and CLI workflow call sites/tests to use renamed metadata constructors and kind constants.
    4. Remove old orchestration `mod` symbols so execution paths use one `mig` vocabulary.
  - Verification:
    1. Run `go test ./internal/workflow/... ./internal/nodeagent ./internal/server/handlers ./internal/cli/...`.
    2. Run `rg -n '\bJobKindMod\b|\bNewModJobMeta\b' internal`.
  - Reasoning: high (8 CFP)

- [x] 1.5 Rename `mods_*.go` files to `migs_*.go` in workflow contracts
  - Type: determined
  - Component: `internal/workflow/contracts/migs_spec.go`, `internal/workflow/contracts/migs_spec_parse.go`, `internal/workflow/contracts/migs_spec_amata_test.go`, `internal/workflow/contracts/migs_spec_tmpdir_test.go`, `internal/workflow/contracts/migs_spec_build_gate_test.go`, `internal/workflow/contracts/migs_spec_healing_test.go`, `internal/workflow/contracts/migs_spec_roundtrip_test.go`, `internal/workflow/contracts/migs_spec_parse_test.go`
  - Implementation:
    1. Rename each `mods_*.go` file in `internal/workflow/contracts` to matching `migs_*.go` filename.
    2. Update references in docs/comments/scripts that point to old `mods_*.go` filenames.
    3. Remove all remaining `mods_*.go` file paths from the repository.
  - Verification:
    1. Run `go test ./internal/workflow/contracts`.
    2. Run `rg --files internal/workflow/contracts | rg 'mods_.*\.go$'`.
    3. Run `rg --files internal/workflow/contracts | rg 'migs_.*\.go$'`.
  - Reasoning: medium (4 CFP)

- [ ] 1.6 Rename overloaded `JobMeta` fields to explicit metadata names
  - Type: determined
  - Component: `internal/workflow/contracts/job_meta.go`, `internal/workflow/**/*.go`, `internal/server/handlers/**/*.go`, `internal/nodeagent/**/*.go`
  - Implementation:
    1. Rename `JobMeta.Gate` to `JobMeta.GateMetadata` and keep existing JSON field tag.
    2. Rename `JobMeta.Recovery` to `JobMeta.RecoveryMetadata` and keep existing JSON field tag.
    3. Update validation, marshal/unmarshal logic, and all consumers to renamed fields.
    4. Delete old field names so metadata intent is explicit in all call sites.
  - Verification:
    1. Run `go test ./internal/workflow/contracts ./internal/workflow/... ./internal/server/handlers ./internal/nodeagent`.
    2. Run `rg -n '\.Gate\b|\.Recovery\b' internal/workflow internal/server internal/nodeagent`.
  - Reasoning: medium (7 CFP)

- [ ] 1.7 Rename ambiguous TUI root list fields to intent-revealing names
  - Type: determined
  - Component: `internal/tui/model_types.go`, `internal/tui/model_*.go`, `internal/tui/view.go`, `internal/tui/*_test.go`
  - Implementation:
    1. Rename `model.ploy` to `model.rootList`.
    2. Rename `model.secondary` to `model.rightPaneList`.
    3. Rename `model.detail` to `model.detailsList`.
    4. Update all TUI state transitions, rendering, and tests to renamed fields.
  - Verification:
    1. Run `go test ./internal/tui/...`.
    2. Run `rg -n '\bploy\b|\bsecondary\b|\bdetail\b' internal/tui`.
  - Reasoning: low (3 CFP)
