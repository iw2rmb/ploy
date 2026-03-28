CLI HTTP Client Contracts

This document describes the current (HEAD) client-side HTTP contracts implemented by the CLI when calling the control-plane API.

## URL building

- The CLI preserves `BaseURL.Path` when building endpoint URLs.
- Do not build endpoint URLs with `ResolveReference(&url.URL{Path: "/v1/..."})`.
- Do not pass a leading-slash segment to `(*url.URL).JoinPath` (for example `"/v1/runs"`).
- Preferred patterns:
  - `BaseURL.JoinPath("v1", ...)`
  - `url.JoinPath(BaseURL.String(), "v1", ...)`

## Response decoding

- JSON responses decoded into typed structs use strict decoding (`json.Decoder.DisallowUnknownFields`) to fail fast on contract drift.
- Response bodies are capped with `io.LimitReader` to prevent unbounded reads.
- Canonical helpers live in the CLI HTTP helper package.

## Error shaping

- On non-2xx responses, the CLI prefers `{ "error": "..." }` (see `docs/api/components/schemas/common.yaml`).
- If the response is not JSON (or has no `error`), the CLI falls back to:
  - trimmed response body, then
  - `resp.Status`.
