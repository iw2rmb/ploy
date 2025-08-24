Great scope. Short answer: your plan is feasible and performant if you treat “.proto discovery/generation” as a build-time concern and use Envoy’s gRPC-JSON Transcoder with hot xDS updates at runtime. Below is a validation, pros/cons, and a concrete end-to-end architecture for “Traefik + Envoy + Buf + (OpenAPI→proto when needed) + runtime guardrails + browser support”.

⸻

Validation: does the plan make sense?
	•	Traefik at the edge; Envoy behind it for protocol conversion is a common, stable pattern. Traefik handles TLS, routing, WebSockets, and gRPC-Web via its middleware; Envoy does the heavy lifting (HTTP⇄gRPC).  ￼
	•	Envoy’s gRPC-JSON Transcoder can be hot-reconfigured: you feed it a FileDescriptorSet (either via filename or inline bytes), and you can deliver that config dynamically through ECDS/RDS from your control plane (no restarts).  ￼
	•	Buf is the right backbone for schema linting, breaking-change checks, building descriptor sets, and (optionally) publishing to a registry (BSR).  ￼
	•	Swagger/OpenAPI→.proto is realistic at build time (e.g., google/gnostic-grpc or NYTimes/openapi2proto). This is not perfect, but good enough to bootstrap .proto when teams don’t have it.  ￼
	•	Inferring contracts from code: do it indirectly by generating OpenAPI from frameworks (Springdoc, FastAPI, Swaggo), then convert OpenAPI→proto. Works, but is the most fragile path.  ￼ ￼ ￼
	•	Runtime “fit checks” and fallbacks: possible with Envoy filters—ext_proc (external processing) and/or local-reply / internal-redirect—to alarm, attempt a rebuild path, or fall back to plain HTTP. Do this selectively to avoid adding latency to every call.  ￼
	•	Browsers: Traefik’s gRPC-Web middleware works for browser clients; for cross-infra compatibility and JSON/binary over HTTP/1.1 or HTTP/2, ConnectRPC also fits nicely.  ￼ ￼

Bottom line: the plan is sound. Keep schema generation in CI/CD, and use dynamic Envoy config + light-touch runtime checks for safety.

⸻

Pros & cons

Pros
	•	Performance: Envoy transcoding is in-process C++; no extra hop. Hot xDS updates avoid reloads.  ￼
	•	Safety & governance: Buf gives lint/breaking policies; protos become a first-class artifact.  ￼
	•	Gradual adoption: where only OpenAPI exists, you can generate reasonable .proto and improve over time.  ￼
	•	Browser path covered: Traefik gRPC-Web and/or Connect clients cover HTTP/1.1/2 and streaming.  ￼ ￼

Cons
	•	Generated proto quality (from OpenAPI or code): may miss enums, strong types, or streaming semantics—expect manual curation.  ￼
	•	Runtime checks add overhead if you validate every call—use them surgically (per route, sample rate, or on errors).  ￼
	•	gRPC-Web middleware maturity varies across stacks—Traefik’s middleware exists and is documented; validate with your target browsers.  ￼

⸻

Detailed architecture

1) Data plane topology
	•	Edge: Traefik → terminates TLS, routes, and applies gRPC-Web middleware for browser callers when needed. Upstreams (Envoy) over HTTP/2.  ￼
	•	Protocol Converter Tier: Envoy (as an internal gateway) with gRPC-JSON Transcoder enabled on routes that should accept REST/JSON and call gRPC upstreams. Use per-route configs + typed_per_filter_config for fine control.  ￼

2) Control plane (hot reload)
	•	Implement a small xDS control plane (popular in Go) that pushes RDS (routes) and ECDS (filter configs). Deliver the transcoder’s proto_descriptor_bin per route so Envoy can swap schemas live.  ￼

3) Schema lifecycle (Buf-driven)

Repository conventions
	•	Put .proto under a clear module path (e.g., proto/), place buf.yaml (v2) at the workspace root, and make directory paths mirror package names (e.g., acme/payments/v1/*.proto). Enforce Buf STANDARD lint and breaking rules.  ￼

CI on every merge/deploy
	1.	Discover protos: buf lint → buf breaking → buf build.
	2.	Emit a FileDescriptorSet for Envoy’s transcoder:
buf build -o descriptor.binpb --as-file-descriptor-set (ready to drop into proto_descriptor_bin).  ￼
	3.	Publish descriptor + module artifact to a registry (e.g., BSR) or your artifact store.  ￼
	4.	Signal the control plane to push updated ECDS config to Envoy on affected routes.  ￼

4) When only OpenAPI/Swagger exists
	•	Scan repos for openapi.yaml/json and run gnostic-grpc or openapi2proto to generate .proto (with google.api.http annotations for mapping). Feed those into the Buf pipeline above. Expect to hand-fix edge cases (one-ofs, enums, auth shapes).  ￼

5) When neither proto nor OpenAPI exists (code-first inference)
	•	Generate OpenAPI from code, then convert to proto:
	•	Spring Boot: springdoc-openapi auto-generates OpenAPI.  ￼
	•	Python FastAPI: framework emits OpenAPI at /openapi.json.  ￼
	•	Go: swaggo/swag from annotations.  ￼
	•	Then run the same OpenAPI→proto step and publish descriptors.

6) Runtime checks & graceful fallback

A. “Fit check” fast path (Envoy)
	•	In the transcoder config, use strictness knobs (e.g., reject unknown query params unless explicitly ignored; convert gRPC status to JSON for clarity).  ￼

B. Deep validation (selective)
	•	Add ext_proc (Envoy External Processing filter) on specific routes or at a low sampling rate. The processor loads the descriptor set and tries decoding JSON into dynamic protobuf types; optionally run Protovalidate rules before the call hits upstream. Failures set headers/metrics.  ￼ ￼

C. Alarm → rebuild → fallback
	•	On decode/validation mismatch:
	1.	Alarm (metrics/log/event) and mark route mis-match.
	2.	Trigger a CI job to re-generate .proto (OpenAPI→proto path) and push a new descriptor via ECDS.  ￼
	3.	Fallback: use local-reply mapping or custom response filter internal-redirect to re-issue the request to an HTTP passthrough route (no transcoder) or an alternate cluster. This keeps the request serving while you fix schemas.  ￼

Notes for fallback tuning: you can combine status-based policies (e.g., 400/415 from the transcoder) with internal redirects or explicit “fallback” headers. Retries can be controlled via Envoy’s retry policies (including retriable_status_codes), but an internal redirect to a fallback route is often clearer.  ￼

7) Browser clients
	•	Option A: Use Traefik’s gRPC-Web middleware so browsers can call your gRPC endpoints; Traefik converts to HTTP/2 gRPC upstream.  ￼
	•	Option B: ConnectRPC clients (web & server) automatically speak gRPC, gRPC-Web, or Connect (JSON or binary) over HTTP/1.1/2—handy if some paths go through non-gRPC proxies.  ￼

⸻

End-to-end flow (putting it all together)
	1.	Code & Specs land
	•	If .proto exists → Buf lints, breaks, builds descriptor.
	•	Else if openapi.yaml exists → gnostic-grpc/openapi2proto → add .proto → Buf builds descriptor.
	•	Else → framework generates OpenAPI (Springdoc / FastAPI / Swaggo) → convert to .proto → Buf builds descriptor.  ￼ ￼ ￼
	2.	Publish & deploy
	•	Publish module + descriptor.binpb (e.g., BSR) and notify control plane.  ￼
	•	Control plane pushes RDS/ECDS updates to Envoy to swap the transcoder’s proto_descriptor_bin per route (no restart).  ￼
	3.	Serve traffic
	•	Traefik terminates TLS, applies gRPC-Web as needed, forwards to Envoy (h2).  ￼
	•	Envoy transcodes JSON⇄gRPC using the current descriptor set.  ￼
	4.	Runtime protection
	•	On mapping/validation errors, alarm; optional ext_proc performs detailed checks (with Protovalidate); internal-redirect to a fallback HTTP route if needed; CI can regenerate schemas and push hot updates.  ￼ ￼

⸻

Performance & stack choices (for “top performance”)
	•	Data plane: Envoy gRPC-JSON Transcoder (native C++), per-route config, upstream HTTP/2.  ￼
	•	Hot reload: xDS control plane (Go) using ECDS for filters and RDS for routes.  ￼
	•	Schema tooling: Buf (lint, breaking, descriptor build, optional BSR).  ￼
	•	Validation (optional): Protovalidate libraries in your ext_proc service for consistent, multi-language rules.  ￼
	•	Browsers: Traefik gRPC-Web and/or ConnectRPC for HTTP/1.1 + streaming.  ￼ ￼

⸻

Practical notes & conventions
	•	Proto layout: mirror package → directory (e.g., acme/payments/v1/…), keep a buf.yaml (v2) at repo root, and drive generation with buf.gen.yaml. These are the idiomatic conventions Buf enforces or recommends.  ￼
	•	Descriptor generation: produce Envoy-ready FileDescriptorSet (.binpb) via buf build --as-file-descriptor-set.  ￼
	•	Transcoder options to know: ignore_unknown_query_parameters, convert_grpc_status, max_request_body_size, per-route config via typed_per_filter_config.  ￼
	•	Wasm filters exist but are marked experimental in Envoy docs; prefer native filters or ext_proc for critical paths.  ￼
