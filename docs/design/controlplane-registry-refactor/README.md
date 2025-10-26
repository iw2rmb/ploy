# Registry HTTP Handler Decomposition

## Why
- `internal/api/httpserver/controlplane_registry.go` is currently 617 LOC (per `git ls-files | xargs wc -l`) and still lumps together manifest, blob, upload, and tag logic, plus helper structs and shared metrics calls, making targeted reviews hard and causing wide diffs for localized changes.
- The monolithic file mixes HTTP routing with registry storage/publisher integrations; touching blob uploads risks modifying manifest behaviour because everything shares the same state and helper functions.
- Splitting the code keeps handlers cohesive, clarifies dependencies (store, transfers, publisher), and lets tests focus on one surface area at a time.

## What to do
1. Keep `controlplane_registry.go` as the thin request router plus shared path/digest helpers so other files can stay focused.
2. Introduce the following focused files under `internal/api/httpserver` (all in package `httpserver`). Each file should start with a short comment describing its focus, and every function should gain a one-line comment restating its role.
   - `registry_manifests.go` — contains `handleRegistryManifest`, `parseOCIManifest`, and `collectDescriptorDigests` along with manifest-specific helpers/constants.
   - `registry_blobs.go` — contains `handleRegistryBlobs`, `handleRegistryBlob`, `handleRegistryUploadStart`, `handleRegistryUploadSession`, and `finalizeRegistryUpload`.
   - `registry_tags.go` — contains `handleRegistryTags`.
   - `registry_metrics.go` — contains `recordRegistryRequest` and `recordRegistryPayload`.
   - `registry_types.go` — contains the registry upload/request structs plus the `ociManifest` and `ociDescriptor` types so the other files share a single definition source.
3. Move code verbatim aside from package/file comments and the new function docstrings; avoid behavioural tweaks or API renames.
4. Update `internal/api/httpserver/controlplane_registry.go` imports once helpers are relocated.
5. Ensure `controlplane_registry_test.go` (and any new registry-focused tests once committed) continue to compile by relying on the existing methods; no test logic updates should be required beyond gofmt/lint.

## Dependencies / Coordination
- No control-plane API schema files or CLI docs change because the HTTP surface is unchanged.
- Shares patterns with `docs/design/scheduler-refactor/README.md`; no additional upstream docs needed.

## How to test
- Run `go test ./internal/api/httpserver` to cover the registry handler tests and guardrail suites.
- Optionally rerun `make test` if other packages were touched while chasing compile issues.
