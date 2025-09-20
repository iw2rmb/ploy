# Build Service

## Key Takeaways
- Coordinates every workstation and VPS build flow: request validation, repo preparation, builder orchestration, and status reporting.
- Provides reusable helpers for artifacts, SBOM/signing, resource limits, and log streaming that the API surface calls directly.
- Encapsulates lane-specific logic (Docker, sandboxed builds, registry checks) so other packages only need to configure and trigger builds.

## Feature Highlights
- Trigger pipeline that normalises requests, persists artifacts, renders orchestrator payloads, uploads inputs, and schedules Nomad builders.
- Builder integration helpers, including sandbox execution, log fetch/stream, resource validation, retry policies, and SBOM/signing hooks.
- Dockerfile generation for each lane and framework, with embeddable templates and diff-friendly rendering utilities.
- Post-build verification: registry provenance, artifact bundle assembly, and HTTP responses tailored for CLI/API clients.

## Files & Folders
- `handler.go` – HTTP surface wiring build requests into the trigger pipeline and returning structured responses.
- `trigger.go` / `trigger_core.go` / `trigger_request.go` – High-level orchestration of trigger stages, request decoding, and overall workflow control.
- `trigger_artifacts_store.go` / `trigger_upload.go` / `trigger_storage.go` / `trigger_utils.go` – Input artifact persistence, upload retries, SeaweedFS integration, and shared helpers.
- `trigger_orchestration.go` – Renders orchestration payloads (Nomad jobs, environment maps) and coordinates submission sequencing.
- `builder_job.go` / `builder_logs_fetch.go` / `logs.go` – Builder job lifecycle, log fetchers, streaming utilities, and structured log conversion.
- `sandbox.go` / `sandbox_run_unit_test.go` – Local sandbox execution support for builder image invocations during validation and unit tests.
- `dockerfile_gen.go` / `dockerfile_pair.go` / `templates/` – Dockerfile generation for per-language builds with paired expected outputs and embedded templates.
- `sbom.go` / `signing.go` / `registry_verify.go` – Supply chain steps: SBOM generation, signature verification, and registry result validation.
- `resources.go` / `resources_test.go` – CPU/memory/disk limit parsing and enforcement shared across trigger and builder paths.
- `repository.go` / `builder_job_test.go` etc. – Git workspace preparation, builder job assertions, and repository utilities used across tests.
- `status.go` / `status_*.go` – Build status calculation, HTTP surface helpers, and response formatting consumed by the API.
