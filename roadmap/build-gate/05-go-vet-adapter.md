# Go Vet Static Check Adapter
- [x] Completed 2025-09-27

## Why / What For
Provide a first-class Go static analysis adapter so the build gate can surface actionable diagnostics for Go repositories without bespoke lane scripting.

## Required Changes
- Implement `buildgate.NewGoVetAdapter` to wrap `go vet` execution with manifest-configurable package scopes and build tag propagation.
- Normalise language aliases (e.g., `go`, `golang`) inside the static check registry so manifests, CLI flags, and adapters line up regardless of casing.
- Parse `go vet` output into `StaticCheckFailure` entries that preserve file/line/column metadata for checkpoint publication.

## Definition of Done
- Go vet adapter returns structured failures with the `govet` rule identifier, honouring `StaticCheckRequest.Options["packages"]` (default `./...`) and optional `Options["tags"]` overrides.
- Static check registry accepts `go`/`golang` aliases so lane defaults, manifest overrides, and CLI skip flags stay consistent.
- Design record documents adapter configuration knobs and links to the roadmap entry.

## Tests
- `internal/workflow/buildgate/go_vet_adapter_test.go` covers option propagation, diagnostic parsing, and error handling with a fake command runner.
- `go test -cover ./...` remains green with ≥90% coverage in `internal/workflow/buildgate`.
