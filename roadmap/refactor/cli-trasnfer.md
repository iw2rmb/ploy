# CLI Transfer Refactor Notes (`internal/cli/transfer`)

- Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs/newtypes, JSON boundaries).
- Merged work item: CLI HTTP boundary behavior (URL building, strict decode, error shaping) is implemented once across `internal/cli/*` (see `roadmap/refactor/scope.md`).

## Type Hardening

- Replace stringly-typed transfer “kinds” and “stages”.
  - `UploadSlotRequest.Stage` and `.Kind` are free-form strings (`internal/cli/transfer/client.go:29`).
  - `DownloadSlotRequest.Kind` is also a free-form string (`internal/cli/transfer/client.go:41`).
  - Solution: define small enums/newtypes (`type TransferKind string`, `type TransferStage string`) with `Validate()` and use them in request/response structs so invalid values fail fast.
- Make `slot_id` a typed identifier.
  - `Slot.ID` is `string` and is interpolated into the URL path (`internal/cli/transfer/client.go:81`).
  - Solution: define `type SlotID string` with validation (non-empty, URL-safe) and use it in `Slot`, `Commit`, and `Abort`.
- Tighten digest typing and validation.
  - Digest is a raw string (`UploadSlotRequest.Digest`, `Slot.Digest`, `CommitRequest.Digest`) (`internal/cli/transfer/client.go:33`, `internal/cli/transfer/client.go:53`, `internal/cli/transfer/client.go:60`).
  - Solution: use a domain newtype for digests (e.g., `type Digest string` with `Validate()` enforcing expected format like `sha256:<hex>`), and validate before sending requests.
- Use domain ID types consistently.
  - Requests/response use `domaintypes.JobID` and `domaintypes.NodeID` already (`internal/cli/transfer/client.go:24`, `internal/cli/transfer/client.go:46`).
  - Solution: keep these typed fields end-to-end and avoid reintroducing raw strings at call sites.

## Streamlining / Simplification

- Remove redundant runtime type switching in `requestSlot`.
  - `requestSlot` takes `payload any` and switches on concrete request type (`internal/cli/transfer/client.go:102`) even though only two methods call it.
  - Solution: replace `requestSlot` with two typed helpers (or keep only the generic `doReq` path) so call sites remain compile-time typed without `any`.
- Reduce duplication between `Commit` and `Abort`.
  - Both validate `slotID`, build an endpoint, and call `doReq` (`internal/cli/transfer/client.go:79`, `internal/cli/transfer/client.go:90`).
  - Solution: centralize “slot action” request building in one helper to ensure consistent validation and error shaping.
- Standardize JSON decoding behavior (merged slice).
  - Implement once per `roadmap/refactor/contracts.md` § "HTTP Boundary Decoding (CLI)" and reuse.

## Likely Bugs / Risks

- Potentially unsafe URL path joining.
  - `reqURL.Path = path.Join(strings.TrimSuffix(base.Path, "/"), strings.TrimPrefix(endpoint, "/"))` (`internal/cli/transfer/client.go:147`) can normalize away path segments (e.g., if endpoint contains `..`) and does not escape path parameters like `slot_id`.
  - Solution: treat `slot_id` as validated `SlotID` (no slashes) and use `url.PathEscape` when interpolating dynamic segments.
- No timeouts unless caller supplies them.
  - The client uses `http.DefaultClient` if none provided (`internal/cli/transfer/client.go:126`); if it has no timeout, transfers can hang.
  - Solution: require an `HTTPClient` with a timeout (or set a default timeout in `transfer.Client` constructor and avoid `http.DefaultClient`).
- Partial error shaping.
  - For non-2xx responses, the client reads the entire body and returns it as an error string (`internal/cli/transfer/client.go:160`), but does not cap size and does not parse structured error JSON.
  - Merged slice: implement error decoding + caps once per `roadmap/refactor/contracts.md` § "HTTP Boundary Decoding (CLI)".
