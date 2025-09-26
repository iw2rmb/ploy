# JetStream Client Wiring
- [x] Done (2025-09-26)

## Why / What For
Move the workflow runner off the in-memory event stub so tickets and checkpoints travel over the real JetStream fabric when available. This closes the loop on the event contract slice and prepares the CLI for the upcoming Grid RPC wiring.

## Required Changes
- Implement a JetStream-backed `runner.EventsClient` that claims tickets from `grid.webhook.<tenant>` and publishes checkpoints to `ploy.workflow.<ticket>.checkpoints`.
- Teach the CLI to instantiate the JetStream client when `JETSTREAM_URL` is set and fall back to the in-memory bus for offline slices.
- Bubble configuration errors (invalid URLs, missing streams) so operators see actionable failures instead of silent stub usage.

## Definition of Done
- `ploy workflow run` connects to JetStream and fails fast when the endpoint is unreachable; removing `JETSTREAM_URL` restores the stub behaviour.
- Checkpoints published through the CLI land on JetStream streams with the existing cache-key payloads.
- Documentation reflects the new behaviour and directs users toward the `JETSTREAM_URL` toggle.

## Tests
- Unit tests for the JetStream client cover ticket claims and checkpoint publishing against an in-process NATS server.
- CLI test exercises the error path when JetStream configuration fails, ensuring diagnostics surface to the user.
- `go test -cover ./...` continues to satisfy repository and runner coverage thresholds.
