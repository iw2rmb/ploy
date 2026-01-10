# Cross-Cutting Contracts (Refactor)

This file tracks *remaining* contract work that is not yet reflected in `docs/` at HEAD.

## Spec Merge Strictness (Server)

- When merging spec JSON, reject invalid JSON and non-object JSON with a 400.
- Do not silently substitute `{}` for invalid inputs.

## HTTP Boundary Decoding (CLI)

- URL building must not drop `BaseURL.Path`:
  - Do not pass leading-slash segments to `(*url.URL).JoinPath` (e.g. `"/v1/..."`).
  - Prefer `url.JoinPath(BaseURL.String(), "v1", ...)` or `BaseURL.JoinPath("v1", ...)`.
- Response decoding should fail fast on contract drift:
  - Prefer `json.Decoder.DisallowUnknownFields()` for JSON responses where practical.
  - Cap response-body reads (including error bodies) with `io.LimitReader`.
- Error shaping should be consistent:
  - Prefer `{ "error": "..." }` when present; otherwise trimmed body; otherwise `resp.Status`.

## Resource Units & Heartbeat

- Heartbeat should be a strict, unit-explicit contract:
  - Integer bytes for memory/disk (`mem_{free,total}_bytes`, `disk_{free,total}_bytes`).
  - Integer millicores/millis for CPU (`cpu_{free,total}_millis`) and validate fit-range.
- Avoid redundant/ambiguous identity fields in the heartbeat body:
  - If `{id}` is in the path, either remove `node_id` from the body or enforce `node_id == {id}`.
