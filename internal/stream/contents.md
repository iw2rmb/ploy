[http.go](http.go) SSE HTTP streaming helpers that serve hub events with resume cursors and optional event filtering.
[http_fuzz_test.go](http_fuzz_test.go) Fuzz tests for SSE HTTP framing, request handling, and streaming edge-case robustness.
[hub.go](hub.go) In-memory per-run event hub that stores history, publishes events, and manages subscriber fan-out.
[hub_backpressure_test.go](hub_backpressure_test.go) Tests subscriber backpressure behavior and overflow handling in hub event delivery.
[hub_benchmark_test.go](hub_benchmark_test.go) Benchmarks hub publish and subscription operations under streaming workloads.
[hub_enriched_test.go](hub_enriched_test.go) Tests enriched log/event payload helpers that include node/job execution metadata.
[hub_publish_test.go](hub_publish_test.go) Tests publish semantics for ordering, validation, and terminal event behavior.
[hub_serve_test.go](hub_serve_test.go) Tests SSE serve loop behavior including resume, done-event shutdown, and context cancellation.
[hub_subscribe_test.go](hub_subscribe_test.go) Tests subscription lifecycle, cursor replay, and cancellation semantics.
[hub_test.go](hub_test.go) Core unit tests for hub stream creation, retention, and state transitions.
[hub_validation_test.go](hub_validation_test.go) Tests validation guards for run IDs, event types, and malformed publish inputs.
