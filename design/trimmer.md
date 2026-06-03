# Java Gradle Trimmer Endpoint and Assemble Loop

## Summary

Expose the Java Gradle log trimmer as a stateless `ployd` HTTP endpoint and use
it from the Spring Boot 2.7 to 3.0 Gradle follow-up flow. The endpoint accepts
Gradle logs and returns compact YAML or JSON evidence. The migration then treats
`assemble` the same way it treats `runtimeClasspath` and `compileClasspath`:
run the command, trim failed output, pass the optimized result to Codex, and
repeat until the command passes or the remaining blocker is external.

## Scope

In scope:

- Add `POST /v1/trimmer/java/gradle` to `ployd`.
- Move Gradle trimming logic behind a reusable package owned by the trimmer
  domain.
- Stop current Build Gates from invoking the trimmer.
- Document the endpoint in OpenAPI and test route coverage.
- Update the Spring Boot 2.7 to 3.0 MIG at
  `/Users/v.v.kovalev/@gitlab/ploy/migs/java/spring-boot-2.7to3.0` so Gradle
  assemble runs as a command/Codex repair loop.
- Before assemble, discover Gradle tasks ending in `TestClasses`, including
  `testClasses`, `integrationTestClasses`, `testDebugUnitTestClasses`, and
  other matching task names, and append them to the assemble invocation.

Out of scope:

- New trimmers for Maven or non-Java tools.
- A generic "run Gradle on the server" service. The endpoint only trims logs.
- Persisting trimmer requests, responses, or logs.
- Changing `runtimeClasspath` or `compileClasspath` audit semantics except for
  sharing endpoint-call helpers where useful.
- Backward-compatible legacy route aliases.

## Why This Is Needed

The Gradle classpath repair flow already uses an explicit cycle: run a command,
shape the command output into compact structured data, let Codex repair from
that data, and run the command again. In the current Spring Boot follow-up flow,
`runtimeClasspath` and `compileClasspath` use this pattern, while assemble is a
single Codex instruction that asks Codex to run and diagnose the build itself.

That makes assemble less deterministic:

- Codex sees raw, noisy Gradle output instead of bounded structured evidence.
- The loop is implicit inside Codex behavior instead of expressed in the MIG.
- Test class compilation tasks are not guaranteed to run before success is
  declared, so migration errors can remain hidden until later test execution.

The existing Gradle trimmer is also only reachable from ploy internals. A
`ployd` endpoint lets other tools post logs and get the same optimized evidence
without embedding ploy workflow code.

## Goals

- Make Java Gradle trimming a reusable server API, not a Build Gate-only helper.
- Keep the endpoint stateless and deterministic: response depends only on the
  submitted log and requested response format.
- Return machine-readable evidence suitable for Amata command responses and
  Codex prompts.
- Preserve the existing compact Gradle evidence schema where possible.
- Express assemble repair as an explicit MIG loop.
- Ensure assemble also compiles discovered test classes.
- Keep full build logs in MIG artifacts while passing only trimmed evidence to
  Codex.
- Keep Build Gate results untrimmed; Build Gates are not consumers of the new
  trimmer endpoint or package.

## Non-goals

- Do not execute Gradle in `ployd`.
- Do not store user logs server-side.
- Do not add compatibility handling for old trimmer response shapes.
- Do not add legacy-specific validation guards.
- Do not make the Spring Boot MIG depend on Build Gate internals.
- Do not preserve Build Gate trimming behavior.

## Current Baseline (Observed)

- Routes are registered from `internal/server/handlers/register.go`; current
  route groups cover health, config, tokens, migs, artifacts, runs, repos,
  nodes, spec bundles, job artifacts, and jobs. No trimmer route exists today.
- OpenAPI paths live under `docs/api/OpenAPI.yaml` and route coverage is
  enforced in `internal/server/handlers/register_routes_coverage_test.go`.
- Handler helpers already provide strict JSON decoding and response writing in
  `internal/server/handlers/ingest_common.go`.
- Gradle trimming logic currently lives in
  `internal/workflow/step/build_gate_log_trimmer.go`. The public entrypoints are
  `TrimGateLog(tool, logText string)` and
  `GateLogFindingContent(tool, logText string)`.
- The Gradle trimmer splits around `* What went wrong:`, preserves compiler
  diagnostics before that block, removes Gradle `* Try:` noise, deduplicates
  repeated stack frames, and can emit structured YAML evidence.
- Build Gate currently calls that trimmer from the workflow step package. This
  design removes that coupling.
- The structured Gradle evidence schema is duplicated in
  `cmd/assets/ployd-node/gradle.java.trimmer.schema.json` and
  `docs/schemas/gradle.java.trimmer.schema.json`.
- The Spring Boot follow-up flow calls `gradle_classpath_audit` for
  `runtimeClasspath` and `compileClasspath`, and that helper recursively calls
  itself after a Codex repair step while failures remain.
- The same follow-up flow currently handles Gradle assemble as one Codex step
  that tells Codex to run
  `./gradlew -q -p /workspace --stacktrace --build-cache assemble` and rerun it
  manually after repairs.
- MIG job manifests receive stack env such as `PLOY_STACK_TOOL` from
  `internal/nodeagent/manifest.go`. The node must also provide `PLOY_SERVER_URL`
  to every MIG container by default so MIG scripts can call ployd-owned utility
  endpoints.

## Target Contract

### Endpoint

`ployd` exposes:

```text
POST /v1/trimmer/java/gradle
```

The endpoint is a stateless utility endpoint. It does not read or mutate the
store, blobstore, events service, or node state.

Accepted request forms:

```http
POST /v1/trimmer/java/gradle?format=json
Content-Type: application/json

{"log":"... Gradle output ..."}
```

```http
POST /v1/trimmer/java/gradle?format=yaml
Content-Type: text/plain

... Gradle output ...
```

Rules:

- `format` is optional and accepts only `json` or `yaml`.
- When `format` is absent, the handler uses `Accept`; if that is inconclusive,
  it returns JSON.
- JSON requests use strict decoding with one required field: `log`.
- Plain text requests treat the entire body as the log.
- Request body size is capped. Use the existing 10 MiB decoded log convention
  unless implementation finds a stricter existing HTTP body cap for utility
  endpoints.
- The endpoint returns `400` for invalid JSON, unknown JSON fields, unsupported
  format, or empty logs.
- The endpoint returns `413` for oversized logs.
- The endpoint must not log request bodies.
- The endpoint must not persist request bodies or responses.

The JSON response shape is:

```json
{
  "tool": "gradle",
  "message": "compact human-readable Gradle failure text",
  "evidence": {
    "task": "compileJava",
    "errors": [
      {
        "message": "cannot find symbol",
        "symbol": "class Example",
        "location": "package com.example",
        "base": "/workspace/src/main/java/com/example/",
        "snippet": "new Example()",
        "files": [{"path": "A.java:10"}]
      }
    ]
  }
}
```

The YAML response encodes the same object:

```yaml
tool: gradle
message: |-
  compact human-readable Gradle failure text
evidence:
  task: compileJava
  errors:
    - message: cannot find symbol
      symbol: class Example
      location: package com.example
      base: /workspace/src/main/java/com/example/
      snippet: new Example()
      files:
        - path: A.java:10
```

If the trimmer cannot build structured evidence, `evidence` is omitted and
`message` contains the best bounded compact log. The endpoint never returns raw
unbounded logs in `message`.

### Package Ownership

Gradle trimming moves out of `internal/workflow/step` into a trimmer-owned
package, for example:

```text
internal/trimmer/java/gradle
```

The package owns:

- Gradle regexes and parsing.
- Response/evidence structs.
- JSON/YAML marshaling helpers used by tests and the endpoint.
- Unit tests for realistic Gradle logs.

`internal/workflow/step` keeps only the Build Gate dispatch boundary. Its
Gradle branch no longer calls the trimmer. Build Gate metadata should expose
untrimmed canonical logs and should not populate trimmer-derived structured
evidence.

### Authentication and Reuse

The trimmer endpoint is not a control-plane data endpoint. It exposes no stored
cluster state and performs no mutation. It can therefore be mounted without a
role requirement so callers that do not use ploy can still post logs and receive
trimmed evidence from a reachable `ployd`.

Deployments still protect the endpoint with the same network/TLS boundary as
the `ployd` listener. The endpoint does not make logs safe to send to an
untrusted server; it only guarantees ployd will not store them.

### Spring Boot MIG Assemble Loop

The Gradle branch in
`/Users/v.v.kovalev/@gitlab/ploy/migs/java/spring-boot-2.7to3.0/followup/amata.yaml`
replaces the single assemble Codex instruction with a helper flow, for example
`gradle_assemble_audit`.

The flow:

1. Selects `./gradlew` when executable, otherwise `gradle`.
2. Discovers additional test-class compilation tasks.
3. Runs assemble plus the discovered tasks.
4. On success, emits a structured pass response and stops.
5. On failure, posts the captured Gradle log to `/v1/trimmer/java/gradle`.
6. Passes the trimmed response to Codex with repair instructions.
7. Recursively calls `gradle_assemble_audit`.

The loop terminates when the command passes or the outer Amata/job budget ends.
The flow keeps the full raw Gradle output in `/out/amata` artifacts and passes
only the endpoint response to Codex.

### Test-Class Task Discovery

Before assemble, the flow discovers task names with Gradle itself:

```sh
"$gradle_cmd" --no-daemon --console=plain -q -p /workspace tasks --all
```

Task extraction rules:

- Parse task names from the first token on task-list lines.
- Strip project prefixes when needed; Gradle can run all tasks matching a task
  name across projects from the root invocation.
- Include `testClasses`.
- Include any task name ending with `TestClasses`, such as
  `integrationTestClasses` and `testDebugUnitTestClasses`.
- Deduplicate task names.
- Sort task names for deterministic command output.
- Never include the typo suffix `TestClassess`; the Gradle task suffix is
  `TestClasses`.
- If task discovery fails, continue with plain `assemble` and let the assemble
  failure path be trimmed and repaired.

The assemble command shape becomes:

```sh
"$gradle_cmd" --no-daemon --console=plain -q -p /workspace --stacktrace --build-cache assemble $test_class_tasks
```

`assemble` remains first so normal build artifacts are requested even when
additional test-class tasks are present.

## Implementation Notes

### Ploy Server

- Add a route group in `internal/server/handlers/register.go`, for example
  `registerTrimmerRoutes`.
- Mount `POST /v1/trimmer/java/gradle` without store dependencies.
- Add `internal/server/handlers/trimmer_java_gradle.go`.
- Use existing `writeJSON`/HTTP error helpers for JSON responses and errors.
- For YAML responses, set `Content-Type: application/x-yaml`.
- Add `Content-Disposition` with `gradle-trimmed.json` or
  `gradle-trimmed.yaml` so HTTP clients can save the response as a file.
- Add `docs/api/paths/trimmer_java_gradle.yaml`.
- Add `/v1/trimmer/java/gradle` to `docs/api/OpenAPI.yaml`.
- Update `docs/api/verify_openapi_test.go` and rely on
  `register_routes_coverage_test.go` to catch code/spec mismatch.

### Shared Trimmer Package

- Move Gradle-specific structs and functions from
  `internal/workflow/step/build_gate_log_trimmer.go` into
  `internal/trimmer/java/gradle`.
- Export a small API:

```go
type Result struct {
    Tool     string    `json:"tool" yaml:"tool"`
    Message  string    `json:"message" yaml:"message"`
    Evidence *Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

func Trim(logText string) Result
```

- Keep the current evidence field names and YAML/JSON shape.
- Keep existing Gradle trimmer unit tests, moving them to the new package or
  adding endpoint tests that prove the same behavior.
- Remove Build Gate calls to `GateLogFindingContent("gradle", log)`.
- Delete or narrow Build Gate trimmer tests so they verify only current Build
  Gate behavior: canonical logs are not trimmed by gate execution.

### Spring Boot MIG

- Add a schema for the assemble audit response in `followup/amata.yaml`.
- Add a helper flow in `followup/amata.yaml` or a separate include file if the
  shell block becomes large.
- Prefer a separate include file only if it keeps the main Spring Boot flow
  readable; otherwise keep the helper next to existing Gradle follow-up logic.
- The command step should print JSON so Amata can validate it with a response
  schema.
- The command step should write raw assemble logs to `/out/amata` before
  trimming, so post-factum debugging still has complete output.
- Codex receives only:
  - command run,
  - exit code,
  - discovered test-class tasks,
  - trimmer response,
  - concise repair rules.
- Codex must edit only under `/workspace`, must not invent dependency versions,
  and must rerun only through the surrounding helper flow.

### Endpoint Call From MIG

The helper flow must call the ployd endpoint through the node-provided server
URL:

```sh
"$PLOY_SERVER_URL/v1/trimmer/java/gradle"
```

Implementation requirements:

- The node manifest builder injects `PLOY_SERVER_URL` into every MIG container.
- MIG scripts must not introduce `PLOY_TRIMMER_JAVA_GRADLE_URL`.
- MIG scripts must not contain local-deployment endpoint discovery.
- If `PLOY_SERVER_URL` is missing, the command step fails with a clear
  configuration error instead of silently using local trimming or a fallback
  endpoint.

## Milestones

### Milestone 1: Shared Trimmer Package

Scope:

- Move Gradle trimming code to `internal/trimmer/java/gradle`.
- Remove Build Gate usage of the trimmer.

Expected results:

- Existing Gradle trimmer tests pass.
- Build Gate code no longer owns or invokes Gradle parsing internals.
- Build Gate failure metadata is not trimmer-derived.

Testable outcome:

- `go test ./internal/trimmer/java/gradle ./internal/workflow/step`

### Milestone 2: Ployd Endpoint

Scope:

- Add the HTTP handler, route registration, OpenAPI path, and tests.

Expected results:

- `POST /v1/trimmer/java/gradle` accepts JSON or plain text logs.
- JSON and YAML responses contain the same trimmed result.
- Oversized and invalid requests fail deterministically.

Testable outcome:

- `go test ./internal/server/handlers ./docs/api`
- Manual `curl` with a sample Gradle log returns compact JSON/YAML.

### Milestone 3: Spring Boot MIG Assemble Loop

Scope:

- Replace the single Gradle assemble Codex instruction with
  `gradle_assemble_audit`.
- Add test-class task discovery before assemble.
- Post failed assemble logs to the trimmer endpoint.

Expected results:

- Assemble uses the same command/trim/Codex/retry pattern as classpath repair.
- `assemble` is run with discovered `*TestClasses` tasks.

Testable outcome:

- A Gradle project with a failing compile/test-class task produces a compact
  trimmer response passed to Codex.
- A Gradle project with `testClasses` and `integrationTestClasses` runs both
  tasks in the assemble command.

## Acceptance Criteria

- `POST /v1/trimmer/java/gradle` exists on `ployd` and is documented in
  OpenAPI.
- The endpoint returns JSON by default and YAML when requested.
- The endpoint response includes a compact `message` and structured `evidence`
  when the log matches known Gradle patterns.
- The endpoint does not persist logs and does not require a run/job context.
- Current Build Gates do not call the trimmer.
- Build Gate tests assert untrimmed gate log behavior where this boundary is
  relevant.
- The Spring Boot Gradle assemble step is no longer a single Codex-only
  instruction; it is an explicit command/trim/Codex/retry flow.
- The assemble command includes discovered `testClasses` and `*TestClasses`
  tasks before declaring success.
- Raw assemble logs remain available in artifacts.
- No legacy endpoint aliases or old response-shape compatibility tests are
  added.

## Risks

- Open unauthenticated utility endpoints can be abused for CPU work. The
  endpoint must keep a strict body cap and avoid expensive non-linear parsing.
- Gradle task listing can fail because the build script itself is broken. The
  flow must continue to assemble and trim that failure instead of stopping at
  discovery.
- `gradle tasks --all` output is text, so parsing must be conservative and
  covered by realistic samples.
- Some multi-project Gradle builds may expose duplicate task names. The command
  should invoke deduplicated task names from the root so Gradle resolves all
  matching tasks.
- Logs may contain secrets. The endpoint must not store request bodies, and
  callers must still treat the target `ployd` as trusted.

## References

- `internal/server/handlers/register.go`
- `internal/server/handlers/ingest_common.go`
- `internal/server/handlers/register_routes_coverage_test.go`
- `docs/api/OpenAPI.yaml`
- `internal/workflow/step/build_gate_log_trimmer.go`
- `docs/build-gate/trimmer.md`
- `docs/schemas/gradle.java.trimmer.schema.json`
- `cmd/assets/ployd-node/gradle.java.trimmer.schema.json`
- `/Users/v.v.kovalev/@gitlab/ploy/migs/java/spring-boot-2.7to3.0/followup/amata.yaml`
- `/Users/v.v.kovalev/@gitlab/ploy/migs/java/spring-boot-2.7to3.0/followup/gradle-classpath.yaml`
