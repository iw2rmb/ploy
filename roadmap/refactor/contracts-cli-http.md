# Contract: HTTP Boundary Decoding (CLI)

This file tracks *remaining* CLI HTTP boundary contract work that is not yet
reflected in `docs/` at HEAD.

## URL Building (BaseURL.Path Preservation)

- URL building must not drop `BaseURL.Path`:
  - Do not pass leading-slash segments to `(*url.URL).JoinPath` (e.g. `"/v1/..."`).
  - Prefer `url.JoinPath(BaseURL.String(), "v1", ...)` or `BaseURL.JoinPath("v1", ...)`.

## Response Decoding (Fail Fast on Contract Drift)

- Prefer `json.Decoder.DisallowUnknownFields()` for JSON responses where practical.
- Cap response-body reads (including error bodies) with `io.LimitReader`.

## Error Shaping

- Prefer `{ "error": "..." }` when present; otherwise trimmed body; otherwise `resp.Status`.

